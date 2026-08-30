package email_server

import (
	"os"
	"path/filepath"
	"testing"

	"redock/cloudflare"
	"redock/platform/memory"
)

// writeMaildir creates a Maildir at dir with one message in cur/.
func writeMaildir(t *testing.T, dir, message string) {
	t.Helper()

	for _, sub := range []string{"cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cur", "1234.M1P1Q1.host:2,S"), []byte(message), 0600); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func TestNormalizeMaildirLayoutLiftsHomeLevel(t *testing.T) {
	mailDir := t.TempDir()

	// docker-mailserver's default layout: the Maildir sits under "home".
	account := filepath.Join(mailDir, "example.com", "alice")
	writeMaildir(t, filepath.Join(account, "home"), "Subject: legacy\r\n\r\nbody")
	// A folder under home must come along too.
	if err := os.MkdirAll(filepath.Join(account, "home", ".Sent", "cur"), 0700); err != nil {
		t.Fatalf("mkdir .Sent: %v", err)
	}

	fixed, err := normalizeMaildirLayout(mailDir)
	if err != nil {
		t.Fatalf("normalizeMaildirLayout: %v", err)
	}
	if fixed != 1 {
		t.Fatalf("expected 1 mailbox rearranged, got %d", fixed)
	}

	if !isMaildirFolder(account) {
		t.Fatal("the account root is not a Maildir after normalisation")
	}
	if _, err := os.Stat(filepath.Join(account, "cur", "1234.M1P1Q1.host:2,S")); err != nil {
		t.Fatalf("the message did not move up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(account, ".Sent", "cur")); err != nil {
		t.Fatalf("the Sent folder did not move up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(account, "home")); !os.IsNotExist(err) {
		t.Error("the empty home directory should have been removed")
	}

	// The store must now be able to read it.
	store := NewMaildirStore(mailDir)
	messages, err := store.List(account, inboxName)
	if err != nil {
		t.Fatalf("List after migration: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected the migrated message to be visible, got %d", len(messages))
	}
}

func TestNormalizeMaildirLayoutLeavesCorrectLayoutAlone(t *testing.T) {
	mailDir := t.TempDir()
	account := filepath.Join(mailDir, "example.com", "alice")
	writeMaildir(t, account, "Subject: already fine\r\n\r\nbody")

	fixed, err := normalizeMaildirLayout(mailDir)
	if err != nil {
		t.Fatalf("normalizeMaildirLayout: %v", err)
	}
	if fixed != 0 {
		t.Fatalf("an already-correct layout must not be touched, got %d changes", fixed)
	}
	if _, err := os.Stat(filepath.Join(account, "cur", "1234.M1P1Q1.host:2,S")); err != nil {
		t.Fatalf("the message was disturbed: %v", err)
	}
}

func TestNormalizeMaildirLayoutIsIdempotent(t *testing.T) {
	mailDir := t.TempDir()
	account := filepath.Join(mailDir, "example.com", "alice")
	writeMaildir(t, filepath.Join(account, "home"), "Subject: legacy\r\n\r\nbody")

	if _, err := normalizeMaildirLayout(mailDir); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	fixed, err := normalizeMaildirLayout(mailDir)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if fixed != 0 {
		t.Fatalf("a second pass must be a no-op, got %d changes", fixed)
	}
}

func TestMoveDirContentsNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(dst, 0700); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("from source"), 0600); err != nil {
		t.Fatalf("write src file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "file"), []byte("already there"), 0600); err != nil {
		t.Fatalf("write dst file: %v", err)
	}

	if err := moveDirContents(src, dst); err != nil {
		t.Fatalf("moveDirContents: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "file"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "already there" {
		t.Fatalf("an existing file was overwritten: %q", data)
	}
}

func TestMigrateConfigEnablesEverything(t *testing.T) {
	dir := t.TempDir()
	db, err := memory.NewDatabase(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer db.Close()

	if err := memory.Register[*EmailServerConfig](db, "email_server_configs"); err != nil {
		t.Fatalf("register: %v", err)
	}
	// A configuration as the container-based setup left it: TLS off, no IMAP.
	if err := memory.Create(db, "email_server_configs", &EmailServerConfig{
		Hostname:  "mail.example.com",
		IsRunning: true,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	report := &MigrationReport{}
	mailDir := filepath.Join(dir, "mail")
	if err := migrateConfig(db, mailDir, report); err != nil {
		t.Fatalf("migrateConfig: %v", err)
	}

	config := memory.FindAll[*EmailServerConfig](db, "email_server_configs")[0]
	if config.DataPath != mailDir {
		t.Errorf("the mail directory was not repointed: %q", config.DataPath)
	}
	if !config.IMAPEnabled || !config.IMAPsEnabled || !config.POP3Enabled || !config.SMTPSEnabled {
		t.Errorf("the retrieval listeners were not enabled: %+v", config)
	}
	if !config.SSLEnabled || !config.STARTTLSRequired {
		t.Error("TLS was not enabled")
	}
	if !config.CheckSPF || !config.CheckDKIM || !config.CheckDMARC {
		t.Error("the inbound checks were not enabled")
	}
	if config.RejectOnDMARCFail {
		t.Error("DMARC rejection must stay off until the operator opts in")
	}
	if config.IsRunning {
		t.Error("the running flag must be cleared so the engine starts cleanly")
	}
	if config.SMTPPort != 25 || config.IMAPPort != 143 {
		t.Errorf("default ports were not applied: %+v", config)
	}
}

func TestFindCloudflareZoneMatchesParentZones(t *testing.T) {
	m := newTestManager(t)
	if err := memory.Register[*cloudflare.CloudflareZone](m.db, "cloudflare_zones"); err != nil {
		t.Fatalf("register zones: %v", err)
	}

	for _, zone := range []*cloudflare.CloudflareZone{
		{ZoneID: "zone-example", Name: "example.com"},
		{ZoneID: "zone-sub", Name: "sub.example.com"},
		{ZoneID: "zone-other", Name: "other.test"},
	} {
		if err := memory.Create(m.db, "cloudflare_zones", zone); err != nil {
			t.Fatalf("create zone: %v", err)
		}
	}

	if zone := m.findCloudflareZone("example.com"); zone == nil || zone.ZoneID != "zone-example" {
		t.Fatalf("exact match failed: %+v", zone)
	}
	// A mail domain under a zone resolves to the most specific zone.
	if zone := m.findCloudflareZone("mail.sub.example.com"); zone == nil || zone.ZoneID != "zone-sub" {
		t.Fatalf("expected the most specific parent zone, got %+v", zone)
	}
	if zone := m.findCloudflareZone("nowhere.test"); zone != nil {
		t.Fatalf("an unrelated domain must not match a zone, got %+v", zone)
	}
	// A zone name must not match a domain that merely ends with the same text.
	if zone := m.findCloudflareZone("notexample.com"); zone != nil {
		t.Fatalf("suffix-only overlap must not match, got %+v", zone)
	}
}

func TestMXHostFallsBackToMailSubdomain(t *testing.T) {
	m := newTestManager(t)
	domain := &EmailDomain{Domain: "example.com"}

	m.config.Hostname = "mail.example.com"
	if got := m.mxHostFor(domain); got != "mail.example.com" {
		t.Errorf("a real hostname should be used as the MX target, got %q", got)
	}

	m.config.Hostname = "redock.localhost"
	if got := m.mxHostFor(domain); got != "mail.example.com" {
		t.Errorf("a .localhost hostname must fall back to mail.<domain>, got %q", got)
	}

	domain.MXRecord = "mx1.example.com"
	if got := m.mxHostFor(domain); got != "mx1.example.com" {
		t.Errorf("an explicit MX record must win, got %q", got)
	}
}
