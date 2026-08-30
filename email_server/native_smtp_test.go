package email_server

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"redock/pkg/security"
	"redock/platform/memory"

	"github.com/emersion/go-msgauth/dkim"
	"golang.org/x/crypto/bcrypt"
)

// newTestManager builds a manager backed by a temporary memory DB and a
// temporary Maildir, configured for the native engine.
func newTestManager(t *testing.T) *EmailManager {
	t.Helper()

	dir := t.TempDir()
	db, err := memory.NewDatabase(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []struct {
		name     string
		register func() error
	}{
		{"email_domains", func() error { return memory.Register[*EmailDomain](db, "email_domains") }},
		{"email_mailboxes", func() error { return memory.Register[*EmailMailbox](db, "email_mailboxes") }},
		{"email_aliases", func() error { return memory.Register[*EmailAlias](db, "email_aliases") }},
		{"email_logs", func() error { return memory.Register[*EmailLog](db, "email_logs") }},
	} {
		if err := table.register(); err != nil {
			t.Fatalf("register %s: %v", table.name, err)
		}
	}

	manager := &EmailManager{
		db:            db,
		dataPath:      dir,
		passwordCache: make(map[string]string),
		config: &EmailServerConfig{
			Hostname:      "mail.example.com",
			DataPath:      filepath.Join(dir, "mail"),
			MaxRecipients: 5,
		},
	}
	applyNativeDefaults(manager.config)
	return manager
}

// seedDomain creates a domain plus a mailbox with a known password.
func seedDomain(t *testing.T, m *EmailManager, domainName, username, password string) (*EmailDomain, *EmailMailbox) {
	t.Helper()

	domain := &EmailDomain{Domain: domainName, Enabled: true}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	mailbox := &EmailMailbox{
		DomainID:    domain.ID,
		Username:    username,
		Email:       username + "@" + domainName,
		Password:    string(hash),
		Enabled:     true,
		IMAPEnabled: true,
		SMTPEnabled: true,
		POP3Enabled: true,
	}
	if err := memory.Create(m.db, "email_mailboxes", mailbox); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}
	if err := m.store().EnsureMailbox(domainName, username); err != nil {
		t.Fatalf("ensure maildir: %v", err)
	}
	return domain, mailbox
}

func TestLookupAccountResolvesMailboxAliasAndCatchAll(t *testing.T) {
	m := newTestManager(t)
	domain, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	if account := m.LookupAccount("Alice@Example.COM"); account == nil || account.Mailbox.ID != mailbox.ID {
		t.Fatal("address lookup must be case-insensitive")
	}
	if account := m.LookupAccount("nobody@example.com"); account != nil {
		t.Fatal("an unknown local address must not resolve")
	}
	if account := m.LookupAccount("alice@elsewhere.test"); account != nil {
		t.Fatal("an address in a foreign domain must not resolve")
	}

	alias := &EmailAlias{DomainID: domain.ID, Alias: "info@example.com", DestinationID: mailbox.ID, Enabled: true}
	if err := memory.Create(m.db, "email_aliases", alias); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	if account := m.LookupAccount("info@example.com"); account == nil || account.Mailbox.ID != mailbox.ID {
		t.Fatal("alias must resolve to its destination mailbox")
	}

	// A disabled alias stops resolving.
	alias.Enabled = false
	if err := memory.Update(m.db, "email_aliases", alias); err != nil {
		t.Fatalf("update alias: %v", err)
	}
	if account := m.LookupAccount("info@example.com"); account != nil {
		t.Fatal("a disabled alias must not resolve")
	}

	domain.CatchAll = mailbox.Email
	if err := memory.Update(m.db, "email_domains", domain); err != nil {
		t.Fatalf("update domain: %v", err)
	}
	if account := m.LookupAccount("anything@example.com"); account == nil || account.Mailbox.ID != mailbox.ID {
		t.Fatal("catch-all must accept unknown local parts")
	}
}

func TestAuthenticateChecksPasswordAndState(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	if _, err := m.Authenticate("alice@example.com", "secret"); err != nil {
		t.Fatalf("valid credentials were rejected: %v", err)
	}
	if _, err := m.Authenticate("alice@example.com", "wrong"); err == nil {
		t.Fatal("a wrong password must be rejected")
	}
	if _, err := m.Authenticate("ghost@example.com", "secret"); err == nil {
		t.Fatal("an unknown mailbox must be rejected")
	}

	mailbox.Enabled = false
	if err := memory.Update(m.db, "email_mailboxes", mailbox); err != nil {
		t.Fatalf("update mailbox: %v", err)
	}
	if _, err := m.Authenticate("alice@example.com", "secret"); err == nil {
		t.Fatal("a disabled mailbox must not authenticate")
	}
}

func TestInboundSessionRefusesToRelay(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	session := &smtpSession{backend: &smtpBackend{manager: m, submission: false}}
	session.from = "sender@outside.test"

	if err := session.Rcpt("alice@example.com", nil); err != nil {
		t.Fatalf("a local recipient must be accepted: %v", err)
	}
	err := session.Rcpt("stranger@elsewhere.test", nil)
	if err == nil {
		t.Fatal("port 25 must refuse to relay to a foreign domain")
	}
	if !strings.Contains(err.Error(), "Relay access denied") {
		t.Fatalf("expected a relay refusal, got %v", err)
	}
}

func TestSubmissionRequiresAuthentication(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	session := &smtpSession{backend: &smtpBackend{manager: m, submission: true}}

	if err := session.Mail("alice@example.com", nil); err == nil {
		t.Fatal("submission without AUTH must be refused")
	}
	if err := session.Rcpt("someone@elsewhere.test", nil); err == nil {
		t.Fatal("RCPT without AUTH must be refused")
	}
}

func TestSubmissionRefusesForeignSenderAddress(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	account := m.LookupAccount(mailbox.Email)
	session := &smtpSession{backend: &smtpBackend{manager: m, submission: true}, account: account}

	if err := session.Mail("alice@example.com", nil); err != nil {
		t.Fatalf("a sender may use their own address: %v", err)
	}
	if err := session.Mail("someone.else@example.com", nil); err == nil {
		t.Fatal("a sender must not be able to forge another address")
	}
}

func TestRecipientLimitIsEnforced(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")
	m.config.MaxRecipients = 2

	session := &smtpSession{backend: &smtpBackend{manager: m, submission: false}}
	for i := 0; i < 2; i++ {
		if err := session.Rcpt("alice@example.com", nil); err != nil {
			t.Fatalf("recipient %d was rejected: %v", i, err)
		}
	}
	if err := session.Rcpt("alice@example.com", nil); err == nil {
		t.Fatal("the recipient limit must be enforced")
	}
}

func TestDKIMSigningProducesAVerifiableSignature(t *testing.T) {
	m := newTestManager(t)
	domain, _ := seedDomain(t, m, "example.com", "alice", "secret")

	if err := m.ensureDomainDKIM(domain); err != nil {
		t.Fatalf("ensureDomainDKIM: %v", err)
	}
	if domain.DKIMPrivateKey == "" || domain.DKIMPublicKey == "" {
		t.Fatal("DKIM key generation produced nothing")
	}

	raw := []byte("From: alice@example.com\r\n" +
		"To: bob@elsewhere.test\r\n" +
		"Subject: hello\r\n" +
		"Date: Mon, 12 Feb 2026 10:00:00 +0300\r\n" +
		"\r\n" +
		"body\r\n")

	signed, err := m.signMessage("example.com", raw)
	if err != nil {
		t.Fatalf("signMessage: %v", err)
	}
	if !bytes.Contains(signed, []byte("DKIM-Signature:")) {
		t.Fatal("the signed message carries no DKIM-Signature header")
	}
	if !bytes.Contains(signed, []byte("d=example.com")) {
		t.Fatal("the signature is not bound to the sending domain")
	}

	// Verification needs DNS, so only check the header parses as a signature.
	if _, err := dkim.Verify(bytes.NewReader(signed)); err != nil {
		t.Fatalf("the signed message is not parseable by a verifier: %v", err)
	}
}

func TestSignMessageWithoutAKeyIsANoOp(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	raw := []byte("From: alice@example.com\r\n\r\nbody")
	signed, err := m.signMessage("example.com", raw)
	if err != nil {
		t.Fatalf("signing without a key must not fail: %v", err)
	}
	if !bytes.Equal(raw, signed) {
		t.Fatal("the message must be left untouched when there is no key")
	}
}

func TestHeaderValueUnfoldsAndDecodes(t *testing.T) {
	raw := []byte("Subject: =?UTF-8?B?TWVyaGFiYQ==?=\r\n" +
		"To: a@b.test,\r\n\td@e.test\r\n" +
		"\r\n" +
		"body")

	if got := headerValue(raw, "Subject"); got != "Merhaba" {
		t.Errorf("encoded subject was not decoded: %q", got)
	}
	if got := headerValue(raw, "To"); got != "a@b.test, d@e.test" {
		t.Errorf("folded header was not unfolded: %q", got)
	}
	if got := headerValue(raw, "Missing"); got != "" {
		t.Errorf("absent header should be empty, got %q", got)
	}
}

func TestNormalizeAddressStripsDisplayNames(t *testing.T) {
	cases := map[string]string{
		"Alice <Alice@Example.COM>": "alice@example.com",
		"  bob@example.com  ":       "bob@example.com",
		"<c@d.test>":                "c@d.test",
	}
	for input, want := range cases {
		if got := normalizeAddress(input); got != want {
			t.Errorf("normalizeAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGroupByDomainBucketsRecipients(t *testing.T) {
	grouped := groupByDomain([]string{"a@x.test", "b@x.test", "c@y.test", "broken"})

	if len(grouped["x.test"]) != 2 {
		t.Errorf("expected 2 recipients at x.test, got %v", grouped["x.test"])
	}
	if len(grouped["y.test"]) != 1 {
		t.Errorf("expected 1 recipient at y.test, got %v", grouped["y.test"])
	}
	if len(grouped) != 2 {
		t.Errorf("an address without a domain must be skipped, got %v", grouped)
	}
}

func TestDeliverLocalWritesToTheMaildir(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	account := m.LookupAccount("alice@example.com")
	if err := m.deliverLocal(account, inboxName, []byte("Subject: hi\r\n\r\nbody"), nil); err != nil {
		t.Fatalf("deliverLocal: %v", err)
	}

	messages, err := m.store().List(account.Base, inboxName)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected the message in INBOX, got %d", len(messages))
	}
}

func TestLogMailEventIsQueryable(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	m.logMailEvent(mailEvent{
		Direction: "in",
		Status:    "delivered",
		From:      "sender@outside.test",
		To:        "alice@example.com",
		Subject:   "hello",
		Service:   "smtp",
		Detail:    "stored in INBOX",
	})

	result, err := m.GetMailLogs(MailLogQuery{})
	if err != nil {
		t.Fatalf("GetMailLogs: %v", err)
	}
	if result.Source != "native" {
		t.Fatalf("native mode must serve stored events, got source %q", result.Source)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	entry := result.Entries[0]
	if entry.Direction != "in" || entry.Status != "delivered" || entry.Service != "smtp" {
		t.Fatalf("event was not stored faithfully: %+v", entry)
	}
	if result.Stats.Incoming != 1 {
		t.Errorf("expected the incoming counter to be 1, got %d", result.Stats.Incoming)
	}

	// Filters apply to stored events too.
	filtered, err := m.GetMailLogs(MailLogQuery{Direction: "out"})
	if err != nil {
		t.Fatalf("GetMailLogs: %v", err)
	}
	if len(filtered.Entries) != 0 {
		t.Errorf("direction filter did not apply: %+v", filtered.Entries)
	}
}

func TestRequiredDNSRecordsCoverTheEssentials(t *testing.T) {
	m := newTestManager(t)
	domain, _ := seedDomain(t, m, "example.com", "alice", "secret")
	if err := m.ensureDomainDKIM(domain); err != nil {
		t.Fatalf("ensureDomainDKIM: %v", err)
	}

	records := m.RequiredDNSRecords(domain)
	kinds := make(map[string]bool)
	for _, record := range records {
		kinds[record.Type+" "+record.Name] = true
	}

	if !kinds["MX example.com"] {
		t.Error("MX record is missing")
	}
	if !kinds["TXT example.com"] {
		t.Error("SPF record is missing")
	}
	if !kinds["TXT mail._domainkey.example.com"] {
		t.Error("DKIM record is missing")
	}
	if !kinds["TXT _dmarc.example.com"] {
		t.Error("DMARC record is missing")
	}
}

// A mailbox whose bcrypt hash was lost — the dashboard used to blank it when
// listing accounts — must be repaired from the encrypted copy rather than
// leaving the user unable to log in.
func TestRepairMissingPasswordHashes(t *testing.T) {
	m := newTestManager(t)
	key, err := security.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	m.encryptionKey = key

	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	// Reproduce the damage: hash gone, encrypted copy intact.
	encrypted, err := security.EncryptAES256GCM("secret", m.encryptionKey)
	if err != nil {
		t.Fatalf("EncryptAES256GCM: %v", err)
	}
	mailbox.Password = ""
	mailbox.PlainPassword = encrypted
	if err := memory.Update(m.db, "email_mailboxes", mailbox); err != nil {
		t.Fatalf("update mailbox: %v", err)
	}

	if _, err := m.Authenticate("alice@example.com", "secret"); err == nil {
		t.Fatal("the damaged mailbox should not authenticate before repair")
	}

	if repaired := m.repairMissingPasswordHashes(); repaired != 1 {
		t.Fatalf("expected 1 mailbox to be repaired, got %d", repaired)
	}

	if _, err := m.Authenticate("alice@example.com", "secret"); err != nil {
		t.Fatalf("the repaired mailbox still cannot authenticate: %v", err)
	}
	if _, err := m.Authenticate("alice@example.com", "wrong"); err == nil {
		t.Fatal("repair must not accept a wrong password")
	}
}

func TestRepairSkipsMailboxesWithNoPasswordAtAll(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "empty", "secret")

	mailbox.Password = ""
	mailbox.PlainPassword = ""
	if err := memory.Update(m.db, "email_mailboxes", mailbox); err != nil {
		t.Fatalf("update mailbox: %v", err)
	}

	if repaired := m.repairMissingPasswordHashes(); repaired != 0 {
		t.Fatalf("there is nothing to repair, got %d", repaired)
	}
}
