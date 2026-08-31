package email_server

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"redock/pkg/security"
	"redock/platform/memory"
)

// Per-mailbox rules applied at local delivery: the quota that decides whether a
// message fits, the forwarding that sends a copy elsewhere, and the auto-reply
// that answers on the owner's behalf. The dashboard has always offered these
// settings; this is what makes them do something.

// ErrOverQuota is returned when a mailbox cannot take any more mail. It is
// reported to the sender as a temporary failure so a message is retried rather
// than lost outright while the owner clears space.
var ErrOverQuota = fmt.Errorf("mailbox is over quota")

// applyDeliveryRules runs a mailbox's rules for one incoming message and
// reports whether a copy should still be stored locally.
func (m *EmailManager) applyDeliveryRules(account *Account, raw []byte, envelopeSender string) (store bool, err error) {
	mailbox := account.Mailbox

	if err := m.checkQuota(account, int64(len(raw))); err != nil {
		return false, err
	}

	store = true

	if forward := strings.TrimSpace(mailbox.ForwardTo); forward != "" {
		m.forwardCopy(account, forward, raw)
		// KeepCopy decides whether forwarding also leaves the message behind.
		store = mailbox.KeepCopy
	}

	if mailbox.AutoReply && strings.TrimSpace(mailbox.AutoReplyMsg) != "" {
		m.sendAutoReply(account, envelopeSender, raw)
	}

	return store, nil
}

// checkQuota refuses a message that would take a mailbox past its limit.
func (m *EmailManager) checkQuota(account *Account, size int64) error {
	quota := account.Mailbox.Quota
	if quota <= 0 {
		return nil // unlimited
	}

	// Quota is configured in megabytes.
	limit := quota * 1024 * 1024
	used := m.mailboxSize(account)
	if used+size <= limit {
		return nil
	}

	m.logMailEvent(mailEvent{
		Direction: "in",
		Status:    "rejected",
		To:        account.Address(),
		Service:   "quota",
		MailboxID: account.Mailbox.ID,
		Detail: fmt.Sprintf("mailbox is full: %d MB used of %d MB",
			used/(1024*1024), quota),
	})
	return ErrOverQuota
}

// mailboxSize adds up every folder of a mailbox.
func (m *EmailManager) mailboxSize(account *Account) int64 {
	folders, err := m.store().ListFolders(account.Base)
	if err != nil {
		return 0
	}

	var total int64
	for _, folder := range folders {
		stats, err := m.store().Stats(account.Base, folder)
		if err != nil {
			continue
		}
		total += stats.Size
	}
	return total
}

// forwardCopy queues a copy of the message to another address.
func (m *EmailManager) forwardCopy(account *Account, forwardTo string, raw []byte) {
	recipients := parseAddressList(forwardTo)
	if len(recipients) == 0 {
		return
	}

	local := make([]string, 0, len(recipients))
	remote := make([]string, 0, len(recipients))
	for _, address := range recipients {
		if target := m.LookupAccount(address); target != nil {
			// A loop back to the same mailbox would forward forever.
			if target.Mailbox.ID == account.Mailbox.ID {
				continue
			}
			local = append(local, address)
			continue
		}
		remote = append(remote, address)
	}

	for _, address := range local {
		target := m.LookupAccount(address)
		if target == nil {
			continue
		}
		if _, err := m.store().Deliver(target.Base, inboxName, raw, nil, time.Now()); err != nil {
			log.Printf("mail: forwarding to %s failed: %v", address, err)
			continue
		}
		m.logMailEvent(mailEvent{
			Direction: "out", Status: "forwarded", From: account.Address(), To: address,
			Service: "forward", MailboxID: account.Mailbox.ID, Size: int64(len(raw)),
			Detail: "forwarded locally",
		})
	}

	if len(remote) == 0 {
		return
	}

	_, senderDomain := splitAddress(account.Address())
	item := &QueueItem{
		// The envelope sender is the forwarding mailbox, so a bounce comes back
		// here rather than to the original sender.
		From:       account.Address(),
		Recipients: remote,
		Domain:     senderDomain,
		Subject:    headerValue(raw, "Subject"),
		MailboxID:  account.Mailbox.ID,
	}
	if err := m.queue().Enqueue(item, raw); err != nil {
		log.Printf("mail: could not queue a forward for %s: %v", account.Address(), err)
		return
	}

	for _, address := range remote {
		m.logMailEvent(mailEvent{
			Direction: "out", Status: "forwarded", From: account.Address(), To: address,
			Service: "forward", QueueID: item.ID, MailboxID: account.Mailbox.ID,
			Size: int64(len(raw)), Detail: "queued for forwarding",
		})
	}
}

// autoReplyMemory remembers who has already been answered, so a correspondent
// is not told about the holiday on every message they send.
type autoReplyMemory struct {
	mu   sync.Mutex
	sent map[string]time.Time
}

// autoReplyInterval is how long before the same sender is answered again.
const autoReplyInterval = 24 * time.Hour

var autoReplies = &autoReplyMemory{sent: make(map[string]time.Time)}

// shouldReply reports whether this pair has gone long enough without a reply.
func (a *autoReplyMemory) shouldReply(mailbox, sender string) bool {
	key := mailbox + "→" + sender

	a.mu.Lock()
	defer a.mu.Unlock()

	if last, ok := a.sent[key]; ok && time.Since(last) < autoReplyInterval {
		return false
	}
	a.sent[key] = time.Now()

	// Keep the map from growing without bound on a busy server.
	if len(a.sent) > 5000 {
		for k, when := range a.sent {
			if time.Since(when) > autoReplyInterval {
				delete(a.sent, k)
			}
		}
	}
	return true
}

// sendAutoReply answers a message on the mailbox owner's behalf, following the
// conventions that stop vacation responders from causing mail loops.
func (m *EmailManager) sendAutoReply(account *Account, envelopeSender string, raw []byte) {
	sender := normalizeAddress(envelopeSender)
	if sender == "" {
		return // a bounce (empty envelope sender) must never be answered
	}
	if strings.EqualFold(sender, account.Address()) {
		return // never answer ourselves
	}
	if isAutomatedMessage(raw) {
		return
	}
	if !autoReplies.shouldReply(account.Address(), sender) {
		return
	}

	subject := headerValue(raw, "Subject")
	if subject == "" {
		subject = "Re: your message"
	} else if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	client := NewSMTPClient(m)
	reply := &EmailMessage{
		From:      account.Address(),
		To:        []string{sender},
		Subject:   subject,
		BodyPlain: account.Mailbox.AutoReplyMsg,
	}

	mime, err := client.buildMIMEMessage(reply)
	if err != nil {
		log.Printf("mail: could not build the auto-reply for %s: %v", account.Address(), err)
		return
	}
	// Mark it so the other side's own responder ignores it.
	mime = append([]byte("Auto-Submitted: auto-replied\r\nX-Auto-Response-Suppress: All\r\n"), mime...)

	if target := m.LookupAccount(sender); target != nil {
		if _, err := m.store().Deliver(target.Base, inboxName, mime, nil, time.Now()); err != nil {
			log.Printf("mail: could not deliver the auto-reply to %s: %v", sender, err)
			return
		}
	} else {
		_, senderDomain := splitAddress(account.Address())
		item := &QueueItem{
			From:       account.Address(),
			Recipients: []string{sender},
			Domain:     senderDomain,
			Subject:    subject,
			MailboxID:  account.Mailbox.ID,
		}
		if err := m.queue().Enqueue(item, mime); err != nil {
			log.Printf("mail: could not queue the auto-reply for %s: %v", account.Address(), err)
			return
		}
	}

	m.logMailEvent(mailEvent{
		Direction: "out", Status: "auto-replied", From: account.Address(), To: sender,
		Subject: subject, Service: "autoreply", MailboxID: account.Mailbox.ID,
		Detail: "automatic reply sent",
	})
}

// isAutomatedMessage recognises the headers that mark mail no responder should
// answer: other auto-replies, list traffic and bounces.
func isAutomatedMessage(raw []byte) bool {
	for _, header := range []string{"Auto-Submitted", "X-Auto-Response-Suppress", "List-Id", "List-Unsubscribe", "Precedence"} {
		value := strings.ToLower(headerValue(raw, header))
		if value == "" {
			continue
		}
		if header == "Auto-Submitted" && value == "no" {
			continue
		}
		if header == "Precedence" && value != "bulk" && value != "list" && value != "junk" {
			continue
		}
		return true
	}
	return false
}

// parseAddressList splits a comma or semicolon separated address list.
func parseAddressList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' })

	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if address := normalizeAddress(field); address != "" {
			out = append(out, address)
		}
	}
	return out
}

// RefreshMailboxUsage recomputes the stored size and message count of every
// mailbox, which is what the dashboard's quota column reads.
func (m *EmailManager) RefreshMailboxUsage() {
	if m.db == nil {
		return
	}

	for _, mailbox := range memory.FindAll[*EmailMailbox](m.db, "email_mailboxes") {
		if mailbox == nil || mailbox.IsDeleted() {
			continue
		}
		account := m.accountFor(mailbox)
		if account == nil {
			continue
		}

		folders, err := m.store().ListFolders(account.Base)
		if err != nil {
			continue
		}

		var size int64
		count := 0
		for _, folder := range folders {
			stats, err := m.store().Stats(account.Base, folder)
			if err != nil {
				continue
			}
			size += stats.Size
			count += int(stats.Messages)
		}

		if mailbox.UsedQuota == size && mailbox.MessageCount == count {
			continue
		}
		mailbox.UsedQuota = size
		mailbox.MessageCount = count
		if err := memory.Update(m.db, "email_mailboxes", mailbox); err != nil {
			log.Printf("mail: could not update usage for %s: %v", mailbox.Email, err)
		}
	}
}

// startUsageRefresh keeps the usage figures current while the server runs.
func (n *NativeServer) startUsageRefresh(stop chan struct{}) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	n.manager.RefreshMailboxUsage()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			n.manager.RefreshMailboxUsage()
		}
	}
}

// ---- alias management ----

// AddAlias creates an alias pointing at a local mailbox. The destination has to
// be a mailbox this server hosts: forwarding to the outside is what a mailbox's
// ForwardTo setting is for, and allowing it here would make the server relay
// mail for an address it does not own.
func (m *EmailManager) AddAlias(alias, destination string, enabled bool) (*EmailAlias, error) {
	alias = normalizeAddress(alias)
	destination = normalizeAddress(destination)

	if alias == "" || !strings.Contains(alias, "@") {
		return nil, fmt.Errorf("the alias must be a full email address")
	}
	if destination == "" {
		return nil, fmt.Errorf("a destination is required")
	}

	_, domainName := splitAddress(alias)
	domain := m.LookupDomain(domainName)
	if domain == nil {
		return nil, fmt.Errorf("%s is not a domain this server hosts", domainName)
	}

	// An alias must not shadow a real mailbox.
	if existing := m.lookupMailbox(alias); existing != nil {
		return nil, fmt.Errorf("%s is already a mailbox", alias)
	}
	for _, entry := range memory.FindAll[*EmailAlias](m.db, "email_aliases") {
		if entry != nil && !entry.IsDeleted() && strings.EqualFold(entry.Alias, alias) {
			return nil, fmt.Errorf("%s is already an alias", alias)
		}
	}

	target := m.lookupMailbox(destination)
	if target == nil {
		return nil, fmt.Errorf("%s is not a mailbox on this server", destination)
	}

	entry := &EmailAlias{
		DomainID:      domain.ID,
		Alias:         alias,
		Destination:   destination,
		DestinationID: target.Mailbox.ID,
		Enabled:       enabled,
	}
	if err := memory.Create(m.db, "email_aliases", entry); err != nil {
		return nil, fmt.Errorf("failed to store the alias: %w", err)
	}

	m.logMailEvent(mailEvent{
		Direction: "system", Status: "alias-created", Service: "directory",
		From: alias, To: destination, Detail: "alias created",
	})
	return entry, nil
}

// UpdateAlias changes an alias's destination or its enabled state.
func (m *EmailManager) UpdateAlias(id uint, destination string, enabled *bool) (*EmailAlias, error) {
	entry, err := memory.FindByID[*EmailAlias](m.db, "email_aliases", id)
	if err != nil || entry == nil {
		return nil, fmt.Errorf("alias not found")
	}

	if destination != "" {
		address := normalizeAddress(destination)
		target := m.lookupMailbox(address)
		if target == nil {
			return nil, fmt.Errorf("%s is not a mailbox on this server", address)
		}
		entry.Destination = address
		entry.DestinationID = target.Mailbox.ID
	}
	if enabled != nil {
		entry.Enabled = *enabled
	}

	if err := memory.Update(m.db, "email_aliases", entry); err != nil {
		return nil, fmt.Errorf("failed to update the alias: %w", err)
	}
	return entry, nil
}

// DeleteAlias removes an alias.
func (m *EmailManager) DeleteAlias(id uint) error {
	entry, err := memory.FindByID[*EmailAlias](m.db, "email_aliases", id)
	if err != nil || entry == nil {
		return fmt.Errorf("alias not found")
	}
	if err := memory.Delete[*EmailAlias](m.db, "email_aliases", id); err != nil {
		return err
	}

	m.logMailEvent(mailEvent{
		Direction: "system", Status: "alias-deleted", Service: "directory",
		From: entry.Alias, Detail: "alias removed",
	})
	return nil
}

// EnsurePostmaster creates the postmaster mailbox for a domain if it is
// missing. DMARC reports and delivery notifications are addressed there by
// convention, and a domain without one rejects them silently.
func (m *EmailManager) EnsurePostmaster(domain *EmailDomain) error {
	address := "postmaster@" + domain.Domain
	if account := m.LookupAccount(address); account != nil {
		return nil
	}

	password, err := security.GenerateSecurePassword(24)
	if err != nil {
		return err
	}

	mailbox, err := m.AddMailbox(domain.ID, "postmaster", password, "Postmaster")
	if err != nil {
		return fmt.Errorf("could not create %s: %w", address, err)
	}

	m.logMailEvent(mailEvent{
		Direction: "system", Status: "mailbox-created", Service: "directory",
		To: mailbox.Email, MailboxID: mailbox.ID,
		Detail: "postmaster mailbox created so reports and bounces have somewhere to go",
	})
	return nil
}
