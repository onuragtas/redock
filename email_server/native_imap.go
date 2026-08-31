package email_server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/backendutil"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/textproto"

	"redock/platform/memory"
)

// imapDelimiter is the hierarchy separator presented to clients. Maildir++
// uses "." on disk; "/" is friendlier and is translated in folderDir.
const imapDelimiter = "/"

// subscriptionFile stores which folders a client asked to subscribe to.
const subscriptionFile = ".redock-subscriptions.json"

// imapBackend is the entry point go-imap calls to authenticate a client.
// It replaces Dovecot: accounts come from the memory DB, mail from Maildir.
type imapBackend struct {
	manager *EmailManager
}

func (b *imapBackend) Login(connInfo *imap.ConnInfo, username, password string) (backend.User, error) {
	account, err := b.manager.Authenticate(username, password)
	if err != nil {
		remoteIP := ""
		if connInfo != nil && connInfo.RemoteAddr != nil {
			remoteIP = hostOf(connInfo.RemoteAddr)
		}
		b.manager.logMailEvent(mailEvent{
			Direction: "system",
			Status:    "auth-failed",
			From:      username,
			RemoteIP:  remoteIP,
			Service:   "imap",
			Detail:    err.Error(),
		})
		b.manager.noteAuthFailure("imap", remoteIP, username)
		return nil, backend.ErrInvalidCredentials
	}
	if account.Mailbox != nil && !account.Mailbox.IMAPEnabled && account.Mailbox.POP3Enabled {
		// IMAP explicitly disabled for this mailbox.
		return nil, backend.ErrInvalidCredentials
	}

	if err := b.manager.store().EnsureMailbox(account.Domain.Domain, account.Mailbox.Username); err != nil {
		return nil, err
	}
	b.manager.recordLogin(account, "imap")

	return &imapUser{manager: b.manager, account: account}, nil
}

// imapUser is one authenticated account's view of its folders.
type imapUser struct {
	manager *EmailManager
	account *Account

	mu sync.Mutex
}

func (u *imapUser) Username() string { return u.account.Address() }

func (u *imapUser) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	folders, err := u.manager.store().ListFolders(u.account.Base)
	if err != nil {
		return nil, err
	}

	subs := u.subscriptions()
	out := make([]backend.Mailbox, 0, len(folders))
	for _, folder := range folders {
		if subscribed && !subs[folder] && !strings.EqualFold(folder, inboxName) {
			continue
		}
		out = append(out, &imapMailbox{user: u, name: folder})
	}
	return out, nil
}

func (u *imapUser) GetMailbox(name string) (backend.Mailbox, error) {
	folders, err := u.manager.store().ListFolders(u.account.Base)
	if err != nil {
		return nil, err
	}
	for _, folder := range folders {
		if strings.EqualFold(folder, name) {
			return &imapMailbox{user: u, name: folder}, nil
		}
	}
	return nil, backend.ErrNoSuchMailbox
}

func (u *imapUser) CreateMailbox(name string) error {
	name = strings.TrimSuffix(name, imapDelimiter)
	if err := u.manager.store().CreateFolder(u.account.Base, name); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return backend.ErrMailboxAlreadyExists
		}
		return err
	}
	return nil
}

func (u *imapUser) DeleteMailbox(name string) error {
	if err := u.manager.store().DeleteFolder(u.account.Base, name); err != nil {
		if os.IsNotExist(err) {
			return backend.ErrNoSuchMailbox
		}
		return err
	}
	u.setSubscribed(name, false)
	return nil
}

func (u *imapUser) RenameMailbox(existingName, newName string) error {
	if err := u.manager.store().RenameFolder(u.account.Base, existingName, newName); err != nil {
		if os.IsNotExist(err) {
			return backend.ErrNoSuchMailbox
		}
		if strings.Contains(err.Error(), "already exists") {
			return backend.ErrMailboxAlreadyExists
		}
		return err
	}
	u.setSubscribed(existingName, false)
	u.setSubscribed(newName, true)
	return nil
}

func (u *imapUser) Logout() error { return nil }

// ---- subscriptions ----

func (u *imapUser) subscriptionPath() string {
	return filepath.Join(u.account.Base, subscriptionFile)
}

func (u *imapUser) subscriptions() map[string]bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	subs := make(map[string]bool)
	data, err := os.ReadFile(u.subscriptionPath())
	if err != nil {
		// Nothing recorded yet: subscribe to the default set so a fresh client
		// sees Sent/Drafts/Trash without hunting for them.
		subs[inboxName] = true
		for _, folder := range DefaultFolders {
			subs[folder] = true
		}
		return subs
	}

	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return subs
	}
	for _, folder := range list {
		subs[folder] = true
	}
	return subs
}

func (u *imapUser) setSubscribed(folder string, subscribed bool) {
	subs := u.subscriptions()
	if subscribed {
		subs[folder] = true
	} else {
		delete(subs, folder)
	}

	list := make([]string, 0, len(subs))
	for name := range subs {
		list = append(list, name)
	}
	sort.Strings(list)

	u.mu.Lock()
	defer u.mu.Unlock()
	if data, err := json.Marshal(list); err == nil {
		_ = os.WriteFile(u.subscriptionPath(), data, 0600)
	}
}

// imapMailbox is one folder of one account.
type imapMailbox struct {
	user *imapUser
	name string
}

func (mbox *imapMailbox) store() *MaildirStore { return mbox.user.manager.store() }
func (mbox *imapMailbox) base() string         { return mbox.user.account.Base }

func (mbox *imapMailbox) Name() string { return mbox.name }

func (mbox *imapMailbox) Info() (*imap.MailboxInfo, error) {
	info := &imap.MailboxInfo{
		Delimiter: imapDelimiter,
		Name:      mbox.name,
	}
	// Advertise special-use attributes so clients put Sent/Trash in the right
	// place instead of creating duplicates.
	switch strings.ToLower(mbox.name) {
	case "sent":
		info.Attributes = []string{imap.SentAttr}
	case "drafts":
		info.Attributes = []string{imap.DraftsAttr}
	case "trash":
		info.Attributes = []string{imap.TrashAttr}
	case "junk":
		info.Attributes = []string{imap.JunkAttr}
	case "archive":
		info.Attributes = []string{imap.ArchiveAttr}
	}
	return info, nil
}

func (mbox *imapMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	stats, err := mbox.store().Stats(mbox.base(), mbox.name)
	if err != nil {
		return nil, err
	}

	status := imap.NewMailboxStatus(mbox.name, items)
	status.Flags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag}
	status.PermanentFlags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag, "\\*"}
	status.UnseenSeqNum = stats.FirstUnseen

	for _, name := range items {
		switch name {
		case imap.StatusMessages:
			status.Messages = stats.Messages
		case imap.StatusUidNext:
			status.UidNext = stats.UIDNext
		case imap.StatusUidValidity:
			status.UidValidity = stats.UIDValidity
		case imap.StatusRecent:
			status.Recent = stats.Recent
		case imap.StatusUnseen:
			status.Unseen = stats.Unseen
		}
	}
	return status, nil
}

func (mbox *imapMailbox) SetSubscribed(subscribed bool) error {
	mbox.user.setSubscribed(mbox.name, subscribed)
	return nil
}

func (mbox *imapMailbox) Check() error { return nil }

func (mbox *imapMailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)

	messages, err := mbox.store().List(mbox.base(), mbox.name)
	if err != nil {
		return err
	}

	for i, msg := range messages {
		seqNum := uint32(i + 1)
		id := seqNum
		if uid {
			id = msg.UID
		}
		if !seqSet.Contains(id) {
			continue
		}

		fetched, err := mbox.fetch(msg, seqNum, items)
		if err != nil {
			continue // a single unreadable message must not fail the whole FETCH
		}
		ch <- fetched
	}
	return nil
}

// fetch builds the IMAP response for one message, reading the body from disk
// only when an item actually needs it.
func (mbox *imapMailbox) fetch(msg MaildirMessage, seqNum uint32, items []imap.FetchItem) (*imap.Message, error) {
	fetched := imap.NewMessage(seqNum, items)

	var raw []byte
	body := func() ([]byte, error) {
		if raw != nil {
			return raw, nil
		}
		data, err := mbox.store().Read(mbox.base(), mbox.name, msg)
		if err != nil {
			return nil, err
		}
		raw = data
		return raw, nil
	}

	headerAndBody := func() (textproto.Header, io.Reader, error) {
		data, err := body()
		if err != nil {
			return textproto.Header{}, nil, err
		}
		reader := bufio.NewReader(bytes.NewReader(data))
		hdr, err := textproto.ReadHeader(reader)
		return hdr, reader, err
	}

	for _, item := range items {
		switch item {
		case imap.FetchEnvelope:
			hdr, _, err := headerAndBody()
			if err != nil {
				return nil, err
			}
			fetched.Envelope, _ = backendutil.FetchEnvelope(hdr)
		case imap.FetchBody, imap.FetchBodyStructure:
			hdr, reader, err := headerAndBody()
			if err != nil {
				return nil, err
			}
			fetched.BodyStructure, _ = backendutil.FetchBodyStructure(hdr, reader, item == imap.FetchBodyStructure)
		case imap.FetchFlags:
			fetched.Flags = msg.Flags
		case imap.FetchInternalDate:
			fetched.InternalDate = msg.Date
		case imap.FetchRFC822Size:
			fetched.Size = uint32(msg.Size)
		case imap.FetchUid:
			fetched.Uid = msg.UID
		default:
			section, err := imap.ParseBodySectionName(item)
			if err != nil {
				continue
			}
			hdr, reader, err := headerAndBody()
			if err != nil {
				return nil, err
			}
			literal, err := backendutil.FetchBodySection(hdr, reader, section)
			if err != nil {
				continue
			}
			fetched.Body[section] = literal
		}
	}

	return fetched, nil
}

func (mbox *imapMailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	messages, err := mbox.store().List(mbox.base(), mbox.name)
	if err != nil {
		return nil, err
	}

	var ids []uint32
	for i, msg := range messages {
		seqNum := uint32(i + 1)

		raw, err := mbox.store().Read(mbox.base(), mbox.name, msg)
		if err != nil {
			continue
		}
		entity, err := message.Read(bytes.NewReader(raw))
		if err != nil && entity == nil {
			continue
		}

		ok, err := backendutil.Match(entity, seqNum, msg.UID, msg.Date, msg.Flags, criteria)
		if err != nil || !ok {
			continue
		}

		if uid {
			ids = append(ids, msg.UID)
		} else {
			ids = append(ids, seqNum)
		}
	}
	return ids, nil
}

func (mbox *imapMailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if date.IsZero() {
		date = time.Now()
	}

	// A client saving its own copy of a message it has just submitted would
	// otherwise leave two: the server files one as the message is accepted.
	// Report success either way — from the client's side the copy is saved.
	if strings.EqualFold(mbox.name, sentName) &&
		mbox.user.manager.sentHasMessageID(mbox.user.account, messageIDOf(raw)) {
		return nil
	}

	_, err = mbox.store().Deliver(mbox.base(), mbox.name, raw, stripFlag(flags, imap.RecentFlag), date)
	return err
}

func (mbox *imapMailbox) UpdateMessagesFlags(uid bool, seqSet *imap.SeqSet, op imap.FlagsOp, flags []string) error {
	messages, err := mbox.store().List(mbox.base(), mbox.name)
	if err != nil {
		return err
	}

	for i, msg := range messages {
		seqNum := uint32(i + 1)
		id := seqNum
		if uid {
			id = msg.UID
		}
		if !seqSet.Contains(id) {
			continue
		}

		updated := backendutil.UpdateFlags(msg.Flags, op, flags)
		if _, err := mbox.store().SetFlags(mbox.base(), mbox.name, msg, stripFlag(updated, imap.RecentFlag)); err != nil {
			return err
		}
	}
	return nil
}

func (mbox *imapMailbox) CopyMessages(uid bool, seqSet *imap.SeqSet, dest string) error {
	messages, err := mbox.store().List(mbox.base(), mbox.name)
	if err != nil {
		return err
	}

	// The destination must exist; IMAP wants a TRYCREATE hint otherwise.
	if _, err := mbox.user.GetMailbox(dest); err != nil {
		return backend.ErrNoSuchMailbox
	}

	for i, msg := range messages {
		seqNum := uint32(i + 1)
		id := seqNum
		if uid {
			id = msg.UID
		}
		if !seqSet.Contains(id) {
			continue
		}
		if _, err := mbox.store().Copy(mbox.base(), mbox.name, dest, msg); err != nil {
			return err
		}
	}
	return nil
}

func (mbox *imapMailbox) Expunge() error {
	messages, err := mbox.store().List(mbox.base(), mbox.name)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		if !hasFlag(msg.Flags, imap.DeletedFlag) {
			continue
		}
		if err := mbox.store().Remove(mbox.base(), mbox.name, msg); err != nil {
			return err
		}
	}
	return nil
}

// recordLogin updates the mailbox's login bookkeeping and writes a log entry,
// which is what the dashboard's "last login" column reads.
func (m *EmailManager) recordLogin(account *Account, service string) {
	if account == nil || account.Mailbox == nil || m.db == nil {
		return
	}

	now := time.Now()
	account.Mailbox.LastLogin = &now
	account.Mailbox.LastActivity = &now
	account.Mailbox.LoginCount++
	if err := memory.Update(m.db, "email_mailboxes", account.Mailbox); err != nil {
		// Not fatal: the session is still valid even if bookkeeping fails.
		fmt.Printf("mail: could not record login for %s: %v\n", account.Address(), err)
	}

	m.logMailEvent(mailEvent{
		Direction: "system",
		Status:    "login",
		From:      account.Address(),
		Service:   service,
		MailboxID: account.Mailbox.ID,
		Detail:    strings.ToUpper(service) + " login",
	})
}
