package email_server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaildirStore owns the on-disk mail storage for the native mail server.
//
// Layout (Maildir++, the same shape Dovecot uses, so an existing mail volume
// can be served without conversion):
//
//	<root>/<domain>/<user>/{cur,new,tmp}        → INBOX
//	<root>/<domain>/<user>/.Sent/{cur,new,tmp}  → Sent
//	<root>/<domain>/<user>/.a.b/…               → nested folder "a/b"
//
// IMAP needs stable UIDs, which Maildir filenames do not provide, so each
// folder carries a small JSON side file mapping message keys to UIDs.
type MaildirStore struct {
	root string

	mu       sync.Mutex
	uidCache map[string]*uidList // folder dir → uid list
}

// uidList is the per-folder UID bookkeeping persisted next to the messages.
type uidList struct {
	UIDValidity uint32            `json:"uid_validity"`
	UIDNext     uint32            `json:"uid_next"`
	Entries     map[string]uint32 `json:"entries"` // message key → UID
}

// MaildirMessage is one stored message as the IMAP/webmail layers see it.
type MaildirMessage struct {
	UID      uint32
	Key      string // stable identity across flag changes
	Filename string // current on-disk name
	Dir      string // "cur" or "new"
	Size     int64
	Flags    []string // IMAP flags (\Seen, \Answered, …)
	Date     time.Time
}

const (
	uidListFile = ".redock-uids.json"
	// DefaultFolders are created for every new mailbox so clients find the
	// special-use folders where they expect them.
	inboxName = "INBOX"
)

// DefaultFolders is the folder set created with every mailbox.
var DefaultFolders = []string{"Sent", "Drafts", "Trash", "Junk", "Archive"}

// NewMaildirStore creates a store rooted at the given directory.
func NewMaildirStore(root string) *MaildirStore {
	return &MaildirStore{
		root:     root,
		uidCache: make(map[string]*uidList),
	}
}

// Root returns the storage root.
func (s *MaildirStore) Root() string { return s.root }

// MailboxPath is the Maildir root of one account.
func (s *MaildirStore) MailboxPath(domain, user string) string {
	return filepath.Join(s.root, strings.ToLower(domain), strings.ToLower(user))
}

// EnsureMailbox creates the account's Maildir plus the default folders.
func (s *MaildirStore) EnsureMailbox(domain, user string) error {
	base := s.MailboxPath(domain, user)
	if err := ensureMaildirDirs(base); err != nil {
		return err
	}
	for _, folder := range DefaultFolders {
		if err := ensureMaildirDirs(folderDir(base, folder)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMailbox deletes an account's entire Maildir.
func (s *MaildirStore) RemoveMailbox(domain, user string) error {
	base := s.MailboxPath(domain, user)
	if base == "" || base == s.root {
		return fmt.Errorf("refusing to remove %q", base)
	}

	s.mu.Lock()
	for dir := range s.uidCache {
		if strings.HasPrefix(dir, base) {
			delete(s.uidCache, dir)
		}
	}
	s.mu.Unlock()

	return os.RemoveAll(base)
}

func ensureMaildirDirs(dir string) error {
	for _, sub := range []string{"tmp", "new", "cur"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return fmt.Errorf("failed to create maildir %s: %w", dir, err)
		}
	}
	return nil
}

// folderDir maps an IMAP folder name to its directory. INBOX is the Maildir
// root; everything else is a Maildir++ dotted directory, with "/" in the IMAP
// name becoming "." on disk.
func folderDir(base, folder string) string {
	if folder == "" || strings.EqualFold(folder, inboxName) {
		return base
	}
	return filepath.Join(base, "."+strings.ReplaceAll(folder, "/", "."))
}

// folderFromDir is the inverse of folderDir for a directory entry name.
func folderFromDir(name string) string {
	return strings.ReplaceAll(strings.TrimPrefix(name, "."), ".", "/")
}

// ListFolders returns the account's folders, INBOX first.
func (s *MaildirStore) ListFolders(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	folders := []string{inboxName}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !strings.HasPrefix(name, ".") || name == "." || name == ".." {
			continue
		}
		// Skip our own metadata and Dovecot's leftovers.
		if !isMaildirFolder(filepath.Join(base, name)) {
			continue
		}
		folders = append(folders, folderFromDir(name))
	}
	sort.Strings(folders[1:])
	return folders, nil
}

func isMaildirFolder(dir string) bool {
	for _, sub := range []string{"cur", "new"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// CreateFolder adds a folder (and its parents).
func (s *MaildirStore) CreateFolder(base, folder string) error {
	if folder == "" || strings.EqualFold(folder, inboxName) {
		return fmt.Errorf("cannot create %q", folder)
	}
	dir := folderDir(base, folder)
	if isMaildirFolder(dir) {
		return fmt.Errorf("folder already exists: %s", folder)
	}
	return ensureMaildirDirs(dir)
}

// DeleteFolder removes a folder and its messages. INBOX cannot be deleted.
func (s *MaildirStore) DeleteFolder(base, folder string) error {
	if folder == "" || strings.EqualFold(folder, inboxName) {
		return fmt.Errorf("cannot delete INBOX")
	}
	dir := folderDir(base, folder)
	if !isMaildirFolder(dir) {
		return os.ErrNotExist
	}

	s.mu.Lock()
	delete(s.uidCache, dir)
	s.mu.Unlock()

	return os.RemoveAll(dir)
}

// RenameFolder moves a folder to a new name.
func (s *MaildirStore) RenameFolder(base, from, to string) error {
	if strings.EqualFold(from, inboxName) {
		return fmt.Errorf("cannot rename INBOX")
	}
	fromDir := folderDir(base, from)
	toDir := folderDir(base, to)
	if !isMaildirFolder(fromDir) {
		return os.ErrNotExist
	}
	if isMaildirFolder(toDir) {
		return fmt.Errorf("folder already exists: %s", to)
	}

	s.mu.Lock()
	delete(s.uidCache, fromDir)
	delete(s.uidCache, toDir)
	s.mu.Unlock()

	return os.Rename(fromDir, toDir)
}

// Deliver writes a message into a folder and returns its assigned UID. The
// message is written to tmp/ first and renamed into place, so a reader never
// sees a partial file — that atomicity is the whole point of Maildir.
func (s *MaildirStore) Deliver(base, folder string, data []byte, flags []string, date time.Time) (uint32, error) {
	dir := folderDir(base, folder)
	if err := ensureMaildirDirs(dir); err != nil {
		return 0, err
	}
	if date.IsZero() {
		date = time.Now()
	}

	key := newMaildirKey()
	tmpPath := filepath.Join(dir, "tmp", key)
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return 0, err
	}

	// A message with no flags at all is "new"; anything else goes straight to
	// cur/ carrying its flag suffix.
	subdir := "cur"
	name := key + ":2," + encodeMaildirFlags(flags)
	if len(flags) == 0 {
		subdir = "new"
		name = key
	}

	finalPath := filepath.Join(dir, subdir, name)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}
	if !date.IsZero() {
		_ = os.Chtimes(finalPath, date, date)
	}

	return s.assignUID(dir, key)
}

// List returns the folder's messages ordered by UID (that is, by arrival).
func (s *MaildirStore) List(base, folder string) ([]MaildirMessage, error) {
	dir := folderDir(base, folder)
	if !isMaildirFolder(dir) {
		return nil, os.ErrNotExist
	}

	messages := make([]MaildirMessage, 0, 64)
	for _, sub := range []string{"new", "cur"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}

			name := entry.Name()
			key, flagPart := splitMaildirName(name)
			flags := decodeMaildirFlags(flagPart)
			if sub == "new" {
				flags = append(flags, imapFlagRecent)
			}

			messages = append(messages, MaildirMessage{
				Key:      key,
				Filename: name,
				Dir:      sub,
				Size:     info.Size(),
				Flags:    flags,
				Date:     info.ModTime(),
			})
		}
	}

	if err := s.fillUIDs(dir, messages); err != nil {
		return nil, err
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].UID < messages[j].UID })
	return messages, nil
}

// Read returns the raw bytes of one message.
func (s *MaildirStore) Read(base, folder string, msg MaildirMessage) ([]byte, error) {
	return os.ReadFile(filepath.Join(folderDir(base, folder), msg.Dir, msg.Filename))
}

// SetFlags rewrites a message's flags by renaming it, moving it out of new/
// when it stops being recent. Returns the message with its new location.
func (s *MaildirStore) SetFlags(base, folder string, msg MaildirMessage, flags []string) (MaildirMessage, error) {
	dir := folderDir(base, folder)
	oldPath := filepath.Join(dir, msg.Dir, msg.Filename)

	newName := msg.Key + ":2," + encodeMaildirFlags(flags)
	newPath := filepath.Join(dir, "cur", newName)

	if oldPath != newPath {
		if err := os.Rename(oldPath, newPath); err != nil {
			return msg, err
		}
	}

	msg.Dir = "cur"
	msg.Filename = newName
	msg.Flags = stripFlag(flags, imapFlagRecent)
	return msg, nil
}

// Move relocates a message to another folder, keeping its flags and giving it
// a UID in the destination.
func (s *MaildirStore) Move(base, from, to string, msg MaildirMessage) (uint32, error) {
	fromDir := folderDir(base, from)
	toDir := folderDir(base, to)
	if err := ensureMaildirDirs(toDir); err != nil {
		return 0, err
	}

	// A fresh key in the destination keeps UID bookkeeping unambiguous.
	key := newMaildirKey()
	name := key + ":2," + encodeMaildirFlags(msg.Flags)
	if err := os.Rename(filepath.Join(fromDir, msg.Dir, msg.Filename), filepath.Join(toDir, "cur", name)); err != nil {
		return 0, err
	}

	s.forgetUID(fromDir, msg.Key)
	return s.assignUID(toDir, key)
}

// Copy duplicates a message into another folder.
func (s *MaildirStore) Copy(base, from, to string, msg MaildirMessage) (uint32, error) {
	data, err := s.Read(base, from, msg)
	if err != nil {
		return 0, err
	}
	return s.Deliver(base, to, data, stripFlag(msg.Flags, imapFlagRecent), msg.Date)
}

// Remove permanently deletes a message.
func (s *MaildirStore) Remove(base, folder string, msg MaildirMessage) error {
	dir := folderDir(base, folder)
	if err := os.Remove(filepath.Join(dir, msg.Dir, msg.Filename)); err != nil {
		return err
	}
	s.forgetUID(dir, msg.Key)
	return nil
}

// FolderStats reports the counts IMAP STATUS and the dashboard need.
type FolderStats struct {
	Messages    uint32
	Recent      uint32
	Unseen      uint32
	FirstUnseen uint32
	UIDNext     uint32
	UIDValidity uint32
	Size        int64
}

// Stats summarises a folder.
func (s *MaildirStore) Stats(base, folder string) (FolderStats, error) {
	messages, err := s.List(base, folder)
	if err != nil {
		return FolderStats{}, err
	}

	dir := folderDir(base, folder)
	list, err := s.loadUIDList(dir)
	if err != nil {
		return FolderStats{}, err
	}

	stats := FolderStats{
		UIDNext:     list.UIDNext,
		UIDValidity: list.UIDValidity,
		Messages:    uint32(len(messages)),
	}
	for i, msg := range messages {
		stats.Size += msg.Size
		if hasFlag(msg.Flags, imapFlagRecent) {
			stats.Recent++
		}
		if !hasFlag(msg.Flags, imapFlagSeen) {
			stats.Unseen++
			if stats.FirstUnseen == 0 {
				stats.FirstUnseen = uint32(i + 1) // sequence number, 1-based
			}
		}
	}
	return stats, nil
}

// ---- UID bookkeeping ----

func (s *MaildirStore) loadUIDList(dir string) (*uidList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUIDListLocked(dir)
}

func (s *MaildirStore) loadUIDListLocked(dir string) (*uidList, error) {
	if list, ok := s.uidCache[dir]; ok {
		return list, nil
	}

	list := &uidList{
		UIDValidity: uint32(time.Now().Unix()),
		UIDNext:     1,
		Entries:     make(map[string]uint32),
	}

	data, err := os.ReadFile(filepath.Join(dir, uidListFile))
	if err == nil {
		if err := json.Unmarshal(data, list); err != nil || list.Entries == nil {
			// Corrupt bookkeeping: start over with a new validity so clients
			// resynchronise instead of trusting stale UIDs.
			list = &uidList{
				UIDValidity: uint32(time.Now().Unix()),
				UIDNext:     1,
				Entries:     make(map[string]uint32),
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if list.UIDNext == 0 {
		list.UIDNext = 1
	}
	s.uidCache[dir] = list
	return list, nil
}

func (s *MaildirStore) saveUIDListLocked(dir string, list *uidList) error {
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, uidListFile+".tmp")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, uidListFile))
}

func (s *MaildirStore) assignUID(dir, key string) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUIDListLocked(dir)
	if err != nil {
		return 0, err
	}
	if uid, ok := list.Entries[key]; ok {
		return uid, nil
	}

	uid := list.UIDNext
	list.Entries[key] = uid
	list.UIDNext++
	return uid, s.saveUIDListLocked(dir, list)
}

// fillUIDs assigns UIDs to any message the bookkeeping has not seen yet (a
// message delivered by another process, or an imported Maildir).
func (s *MaildirStore) fillUIDs(dir string, messages []MaildirMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUIDListLocked(dir)
	if err != nil {
		return err
	}

	// Newcomers get UIDs in arrival order so sequence numbers stay sensible.
	missing := make([]int, 0, len(messages))
	for i := range messages {
		if uid, ok := list.Entries[messages[i].Key]; ok {
			messages[i].UID = uid
			continue
		}
		missing = append(missing, i)
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Slice(missing, func(a, b int) bool {
		return messages[missing[a]].Date.Before(messages[missing[b]].Date)
	})
	for _, i := range missing {
		uid := list.UIDNext
		list.Entries[messages[i].Key] = uid
		messages[i].UID = uid
		list.UIDNext++
	}

	// Drop bookkeeping for messages that no longer exist so the file cannot
	// grow without bound.
	present := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		present[msg.Key] = struct{}{}
	}
	for key := range list.Entries {
		if _, ok := present[key]; !ok {
			delete(list.Entries, key)
		}
	}

	return s.saveUIDListLocked(dir, list)
}

func (s *MaildirStore) forgetUID(dir, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadUIDListLocked(dir)
	if err != nil {
		return
	}
	delete(list.Entries, key)
	_ = s.saveUIDListLocked(dir, list)
}

// ---- filename helpers ----

var maildirCounter struct {
	sync.Mutex
	n uint32
}

// newMaildirKey builds a filename unique on this host, per the Maildir spec.
func newMaildirKey() string {
	maildirCounter.Lock()
	maildirCounter.n++
	counter := maildirCounter.n
	maildirCounter.Unlock()

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "redock"
	}
	host = strings.NewReplacer("/", "\\057", ":", "\\072").Replace(host)

	now := time.Now()
	return fmt.Sprintf("%d.M%dP%dQ%d.%s", now.Unix(), now.Nanosecond()/1000, os.Getpid(), counter, host)
}

// splitMaildirName separates the stable key from the ":2,flags" suffix.
func splitMaildirName(name string) (key, flags string) {
	if idx := strings.LastIndex(name, ":2,"); idx >= 0 {
		return name[:idx], name[idx+3:]
	}
	return name, ""
}

// IMAP system flags used across the native server.
const (
	imapFlagSeen     = "\\Seen"
	imapFlagAnswered = "\\Answered"
	imapFlagFlagged  = "\\Flagged"
	imapFlagDeleted  = "\\Deleted"
	imapFlagDraft    = "\\Draft"
	imapFlagRecent   = "\\Recent"
)

// maildirFlagMap maps Maildir flag letters to IMAP flags.
var maildirFlagMap = []struct {
	letter byte
	flag   string
}{
	{'S', imapFlagSeen},
	{'R', imapFlagAnswered},
	{'F', imapFlagFlagged},
	{'T', imapFlagDeleted},
	{'D', imapFlagDraft},
}

func decodeMaildirFlags(letters string) []string {
	flags := make([]string, 0, len(letters))
	for i := 0; i < len(letters); i++ {
		for _, m := range maildirFlagMap {
			if letters[i] == m.letter {
				flags = append(flags, m.flag)
			}
		}
	}
	return flags
}

func encodeMaildirFlags(flags []string) string {
	var b strings.Builder
	// Maildir requires the letters in ASCII order, which maildirFlagMap is not,
	// so iterate the canonical order explicitly.
	for _, letter := range []byte{'D', 'F', 'R', 'S', 'T'} {
		for _, m := range maildirFlagMap {
			if m.letter != letter {
				continue
			}
			if hasFlag(flags, m.flag) {
				b.WriteByte(letter)
			}
		}
	}
	return b.String()
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

func stripFlag(flags []string, flag string) []string {
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		if !strings.EqualFold(f, flag) {
			out = append(out, f)
		}
	}
	return out
}
