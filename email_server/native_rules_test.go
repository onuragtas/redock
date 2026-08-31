package email_server

import (
	"strings"
	"testing"
	"time"

	"redock/platform/memory"
)

func TestQuotaRejectsWhenFull(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	// A one-megabyte quota with a message already stored close to it.
	mailbox.Quota = 1
	if err := memory.Update(m.db, "email_mailboxes", mailbox); err != nil {
		t.Fatalf("update mailbox: %v", err)
	}

	account := m.LookupAccount(mailbox.Email)
	big := make([]byte, 900*1024)
	for i := range big {
		big[i] = 'x'
	}
	if _, err := m.store().Deliver(account.Base, inboxName, big, nil, time.Now()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	// A second large message no longer fits.
	if _, err := m.applyDeliveryRules(account, big, "sender@outside.test"); err != ErrOverQuota {
		t.Fatalf("expected the mailbox to be over quota, got %v", err)
	}

	// With no quota set, anything fits.
	mailbox.Quota = 0
	_ = memory.Update(m.db, "email_mailboxes", mailbox)
	account = m.LookupAccount(mailbox.Email)
	if store, err := m.applyDeliveryRules(account, big, "sender@outside.test"); err != nil || !store {
		t.Fatalf("an unlimited mailbox must accept mail: store=%v err=%v", store, err)
	}
}

func TestForwardingKeepsOrDropsTheLocalCopy(t *testing.T) {
	m := newTestManager(t)
	_, alice := seedDomain(t, m, "example.com", "alice", "secret")

	// A second mailbox in the same domain to forward to.
	bob := &EmailMailbox{DomainID: alice.DomainID, Username: "bob", Email: "bob@example.com", Enabled: true}
	if err := memory.Create(m.db, "email_mailboxes", bob); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}
	if err := m.store().EnsureMailbox("example.com", "bob"); err != nil {
		t.Fatalf("ensure maildir: %v", err)
	}

	alice.ForwardTo = "bob@example.com"
	alice.KeepCopy = true
	_ = memory.Update(m.db, "email_mailboxes", alice)

	account := m.LookupAccount(alice.Email)
	raw := []byte("Subject: forwarded\r\n\r\nbody")

	store, err := m.applyDeliveryRules(account, raw, "sender@outside.test")
	if err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}
	if !store {
		t.Error("KeepCopy is on, so the original must still be stored")
	}

	bobAccount := m.LookupAccount("bob@example.com")
	forwarded, _ := m.store().List(bobAccount.Base, inboxName)
	if len(forwarded) != 1 {
		t.Fatalf("the forward did not arrive: %d messages", len(forwarded))
	}

	// With KeepCopy off, the original is not stored.
	alice.KeepCopy = false
	_ = memory.Update(m.db, "email_mailboxes", alice)
	account = m.LookupAccount(alice.Email)

	store, err = m.applyDeliveryRules(account, raw, "sender@outside.test")
	if err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}
	if store {
		t.Error("with KeepCopy off the message should only be forwarded")
	}
}

func TestForwardingDoesNotLoopBackToItself(t *testing.T) {
	m := newTestManager(t)
	_, alice := seedDomain(t, m, "example.com", "alice", "secret")

	alice.ForwardTo = alice.Email // a mistake a user can easily make
	alice.KeepCopy = false
	_ = memory.Update(m.db, "email_mailboxes", alice)

	account := m.LookupAccount(alice.Email)
	if _, err := m.applyDeliveryRules(account, []byte("Subject: loop\r\n\r\nbody"), "sender@outside.test"); err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}

	messages, _ := m.store().List(account.Base, inboxName)
	if len(messages) != 0 {
		t.Fatalf("forwarding to itself must not deliver a copy, found %d", len(messages))
	}
	if len(m.QueueItems()) != 0 {
		t.Fatal("forwarding to itself must not queue anything")
	}
}

func TestAutoReplyAnswersOncePerSender(t *testing.T) {
	m := newTestManager(t)
	_, alice := seedDomain(t, m, "example.com", "alice", "secret")

	alice.AutoReply = true
	alice.AutoReplyMsg = "I am away until Monday."
	_ = memory.Update(m.db, "email_mailboxes", alice)

	// The reply goes to a local mailbox so the test can read it.
	bob := &EmailMailbox{DomainID: alice.DomainID, Username: "bob", Email: "bob@example.com", Enabled: true}
	_ = memory.Create(m.db, "email_mailboxes", bob)
	_ = m.store().EnsureMailbox("example.com", "bob")

	account := m.LookupAccount(alice.Email)
	raw := []byte("Subject: question\r\nFrom: bob@example.com\r\n\r\nbody")

	if _, err := m.applyDeliveryRules(account, raw, "bob@example.com"); err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}

	bobAccount := m.LookupAccount("bob@example.com")
	replies, _ := m.store().List(bobAccount.Base, inboxName)
	if len(replies) != 1 {
		t.Fatalf("expected one auto-reply, got %d", len(replies))
	}

	body, _ := m.store().Read(bobAccount.Base, inboxName, replies[0])
	if !strings.Contains(string(body), "away until Monday") {
		t.Errorf("the auto-reply body is wrong: %q", body)
	}
	if !strings.Contains(string(body), "Auto-Submitted: auto-replied") {
		t.Error("an auto-reply must be marked so the other side does not answer it back")
	}

	// A second message from the same sender must not produce another reply.
	if _, err := m.applyDeliveryRules(account, raw, "bob@example.com"); err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}
	replies, _ = m.store().List(bobAccount.Base, inboxName)
	if len(replies) != 1 {
		t.Fatalf("the responder answered twice in a row: %d replies", len(replies))
	}
}

func TestAutoReplyIgnoresBouncesAndAutomatedMail(t *testing.T) {
	m := newTestManager(t)
	_, alice := seedDomain(t, m, "example.com", "alice", "secret")
	alice.AutoReply = true
	alice.AutoReplyMsg = "away"
	_ = memory.Update(m.db, "email_mailboxes", alice)

	account := m.LookupAccount(alice.Email)

	// An empty envelope sender is a bounce: answering it would start a loop.
	if _, err := m.applyDeliveryRules(account, []byte("Subject: bounce\r\n\r\nbody"), ""); err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}
	if len(m.QueueItems()) != 0 {
		t.Fatal("a bounce must never be auto-replied to")
	}

	// Nor should list or auto-generated mail be answered.
	automated := []byte("Subject: newsletter\r\nList-Id: <news.example.com>\r\n\r\nbody")
	if _, err := m.applyDeliveryRules(account, automated, "news@example.net"); err != nil {
		t.Fatalf("applyDeliveryRules: %v", err)
	}
	if len(m.QueueItems()) != 0 {
		t.Fatal("list mail must not be auto-replied to")
	}
}

func TestAliasLifecycle(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	alias, err := m.AddAlias("info@example.com", mailbox.Email, true)
	if err != nil {
		t.Fatalf("AddAlias: %v", err)
	}

	// The delivery path must now resolve it.
	if account := m.LookupAccount("info@example.com"); account == nil || account.Mailbox.ID != mailbox.ID {
		t.Fatal("the alias does not resolve to its destination")
	}

	// Duplicates and shadowing a real mailbox are refused.
	if _, err := m.AddAlias("info@example.com", mailbox.Email, true); err == nil {
		t.Error("a duplicate alias must be refused")
	}
	if _, err := m.AddAlias(mailbox.Email, mailbox.Email, true); err == nil {
		t.Error("an alias must not shadow a real mailbox")
	}
	if _, err := m.AddAlias("info@elsewhere.test", mailbox.Email, true); err == nil {
		t.Error("an alias in a foreign domain must be refused")
	}
	if _, err := m.AddAlias("sales@example.com", "nobody@example.net", true); err == nil {
		t.Error("an alias pointing outside the server must be refused")
	}

	// Disabling stops resolution.
	disabled := false
	if _, err := m.UpdateAlias(alias.ID, "", &disabled); err != nil {
		t.Fatalf("UpdateAlias: %v", err)
	}
	if account := m.LookupAccount("info@example.com"); account != nil {
		t.Error("a disabled alias must not resolve")
	}

	if err := m.DeleteAlias(alias.ID); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if _, err := m.UpdateAlias(alias.ID, "", nil); err == nil {
		t.Error("a deleted alias should no longer be found")
	}
}

func TestEnsurePostmasterCreatesTheMailboxOnce(t *testing.T) {
	m := newTestManager(t)
	domain, _ := seedDomain(t, m, "example.com", "alice", "secret")

	if err := m.EnsurePostmaster(domain); err != nil {
		t.Fatalf("EnsurePostmaster: %v", err)
	}

	account := m.LookupAccount("postmaster@example.com")
	if account == nil {
		t.Fatal("the postmaster mailbox was not created")
	}
	if account.Mailbox.Password == "" {
		t.Error("the postmaster mailbox needs a password so it can be logged into")
	}

	// Running again must not create a second one.
	if err := m.EnsurePostmaster(domain); err != nil {
		t.Fatalf("second EnsurePostmaster: %v", err)
	}
	boxes := memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return mb.Email == "postmaster@example.com"
	})
	if len(boxes) != 1 {
		t.Fatalf("expected exactly one postmaster mailbox, found %d", len(boxes))
	}
}
