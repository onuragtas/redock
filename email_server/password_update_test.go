package email_server

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Setting a password has to restore both halves: the bcrypt hash logins are
// checked against, and the encrypted copy the dashboard reads back. A mailbox
// left with only one of them still counts as broken, which is what the
// Mailboxes tab warns about.
func TestUpdatingAPasswordRestoresBothHalves(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	mailbox, err := m.AddMailbox(domain.ID, "ali", "first-password-1", "Ali")
	if err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}

	// Reproduce the damaged state the warning is there to report.
	mailbox.Password = ""
	mailbox.PlainPassword = ""

	const replacement = "second-password-2"
	if err := m.UpdateMailboxPassword(mailbox.ID, replacement); err != nil {
		t.Fatalf("UpdateMailboxPassword: %v", err)
	}

	if mailbox.Password == "" {
		t.Error("no hash was stored, so nobody can log in")
	}
	if mailbox.PlainPassword == "" {
		t.Error("no encrypted copy was stored, so the mailbox still reads as broken")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(mailbox.Password), []byte(replacement)); err != nil {
		t.Errorf("the stored hash does not match the new password: %v", err)
	}

	// And the account really authenticates with it now.
	if account := m.LookupAccount("ali@example.com"); account == nil {
		t.Fatal("the account disappeared")
	}
	if _, err := m.Authenticate("ali@example.com", replacement); err != nil {
		t.Errorf("the new password does not authenticate: %v", err)
	}
	if _, err := m.Authenticate("ali@example.com", "first-password-1"); err == nil {
		t.Error("the old password still authenticates")
	}
}
