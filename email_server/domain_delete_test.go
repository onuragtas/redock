package email_server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"redock/platform/memory"
)

func domainMailboxes(m *EmailManager, domainID uint) []*EmailMailbox {
	return memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return mb != nil && mb.DomainID == domainID
	})
}

// Adding a domain creates its postmaster mailbox. Counting that against the
// domain made every freshly added domain undeletable, and left the person
// looking for a mailbox they never created.
func TestAFreshDomainCanBeDeleted(t *testing.T) {
	m := newTestManager(t)

	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	if boxes := domainMailboxes(m, domain.ID); len(boxes) != 1 || boxes[0].Username != postmasterUser {
		t.Fatalf("expected only an auto-created postmaster, got %v", boxes)
	}

	if err := m.DeleteDomain(domain.ID); err != nil {
		t.Fatalf("a domain nobody added a mailbox to could not be deleted: %v", err)
	}

	// The postmaster goes with it rather than being orphaned.
	if boxes := domainMailboxes(m, domain.ID); len(boxes) != 0 {
		t.Errorf("%d mailboxes were left behind: %v", len(boxes), boxes)
	}
	if _, err := memory.FindByID[*EmailDomain](m.db, "email_domains", domain.ID); err == nil {
		t.Error("the domain is still there")
	}
}

// A mailbox someone created is theirs to remove; deleting the domain must not
// take their mail with it silently.
func TestDomainWithRealMailboxesIsRefusedByName(t *testing.T) {
	m := newTestManager(t)

	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if _, err := m.AddMailbox(domain.ID, "ali", "secret-password-1", "Ali"); err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}

	err = m.DeleteDomain(domain.ID)
	if err == nil {
		t.Fatal("a domain with a real mailbox was deleted")
	}
	// The message has to say which mailbox, or there is nothing to act on.
	if !strings.Contains(err.Error(), "ali@example.com") {
		t.Errorf("the refusal does not name the mailbox: %v", err)
	}

	// Removing it makes the domain deletable, postmaster and all.
	for _, mb := range domainMailboxes(m, domain.ID) {
		if mb.Username == "ali" {
			if err := m.DeleteMailbox(mb.ID); err != nil {
				t.Fatalf("DeleteMailbox: %v", err)
			}
		}
	}
	if err := m.DeleteDomain(domain.ID); err != nil {
		t.Fatalf("DeleteDomain after clearing: %v", err)
	}
}

// Deleting a domain must not leave its mail on disk.
func TestDeletingADomainRemovesItsMailDirectory(t *testing.T) {
	m := newTestManager(t)

	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	path := filepath.Join(m.config.DataPath, domain.Domain)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("the domain directory was never created here: %v", err)
	}

	if err := m.DeleteDomain(domain.ID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s survived the deletion", path)
	}
}

// Deleting a domain must not leave its aliases behind naming a domain that is
// no longer served.
func TestDeletingADomainTakesItsAliases(t *testing.T) {
	m := newTestManager(t)

	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if _, err := m.AddAlias("info@example.com", "postmaster@example.com", true); err != nil {
		t.Fatalf("AddAlias: %v", err)
	}

	if err := m.DeleteDomain(domain.ID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}

	left := memory.Filter[*EmailAlias](m.db, "email_aliases", func(a *EmailAlias) bool { return a != nil })
	if len(left) != 0 {
		t.Errorf("%d aliases were orphaned: %+v", len(left), left)
	}
}

// The same for a single mailbox: its rules and the aliases pointing at it go
// with it, so mail is never accepted for an address with nowhere to land.
func TestDeletingAMailboxTakesItsRulesAndAliases(t *testing.T) {
	m := newTestManager(t)

	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	mailbox, err := m.AddMailbox(domain.ID, "ali", "secret-password-1", "Ali")
	if err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}

	if _, err := m.AddFilter(&EmailFilter{
		MailboxID:  mailbox.ID,
		Name:       "newsletters",
		Enabled:    true,
		Conditions: `[{"field":"from","operator":"contains","value":"news"}]`,
		Actions:    `[{"type":"move_to","folder":"Archive"}]`,
	}); err != nil {
		t.Fatalf("AddFilter: %v", err)
	}
	if _, err := m.AddAlias("sales@example.com", "ali@example.com", true); err != nil {
		t.Fatalf("AddAlias: %v", err)
	}

	if err := m.DeleteMailbox(mailbox.ID); err != nil {
		t.Fatalf("DeleteMailbox: %v", err)
	}

	if rules := m.ListFilters(mailbox.ID); len(rules) != 0 {
		t.Errorf("%d filter rules were orphaned", len(rules))
	}
	for _, alias := range memory.Filter[*EmailAlias](m.db, "email_aliases", func(a *EmailAlias) bool { return a != nil }) {
		if alias.Alias == "sales@example.com" {
			t.Errorf("the alias pointing at the deleted mailbox is still there: %+v", alias)
		}
	}
}
