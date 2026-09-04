package email_server

import (
	"strings"
	"testing"
)

// A From header carrying only a bare address is one of the things a spam
// filter counts against a message. The mailbox already knows the person's
// name, so there is no reason to leave it out.
func TestFromHeaderCarriesTheDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		display string
		address string
		want    string
	}{
		{"a plain name", "Onur", "onur@example.com", `"Onur" <onur@example.com>`},
		// A comma would otherwise split the header into two addresses.
		{"a name with a comma", "Doe, Jane", "jane@example.com", `"Doe, Jane" <jane@example.com>`},
		// Non-ASCII has to be encoded, not sent raw.
		{"a name with Turkish letters", "Onur Ağtaş", "onur@example.com", "=?utf-8?q?"},
		{"no name at all", "", "onur@example.com", "onur@example.com"},
		{"only whitespace", "   ", "onur@example.com", "onur@example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatFrom(tc.display, tc.address)
			if !strings.Contains(got, tc.want) {
				t.Errorf("formatFrom(%q, %q) = %q, want it to contain %q", tc.display, tc.address, got, tc.want)
			}
			// Whatever the display name, the address must survive intact.
			if !strings.Contains(got, tc.address) {
				t.Errorf("formatFrom(%q, %q) = %q, lost the address", tc.display, tc.address, got)
			}
		})
	}
}

// The header is for people; the envelope is for servers. A display name must
// never leak into the address used for routing or SPF.
func TestTheEnvelopeKeepsTheBareAddress(t *testing.T) {
	header := formatFrom("Onur Ağtaş", "onur@example.com")
	if got := normalizeAddress(header); got != "onur@example.com" {
		t.Errorf("normalizeAddress(%q) = %q, want the bare address", header, got)
	}
}

// The built message must carry the name in From while staying parseable.
func TestBuiltMessageHasANamedFrom(t *testing.T) {
	m := newTestManager(t)
	client := NewSMTPClient(m)

	raw, err := client.buildMIMEMessage(&EmailMessage{
		From: "onur@example.com", FromName: "Onur Ağtaş",
		To: []string{"someone@example.com"}, Subject: "hello", BodyPlain: "hi",
	})
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}

	from := headerValue(raw, "From")
	if !strings.Contains(from, "onur@example.com") {
		t.Fatalf("From = %q, want it to hold the address", from)
	}
	if from == "onur@example.com" {
		t.Error("From is still a bare address; the display name was dropped")
	}
}

// Gmail refuses a message with no From header outright:
//
//	550 5.7.1 Messages missing a valid address in From: header, or having no
//	From: header, are not accepted.
//
// The send itself looked fine — the message was queued and the bounce arrived
// from Google minutes later. SendEmail already knows which mailbox it is
// sending through, so a caller that does not set a sender gets one.
func TestSendEmailFillsInTheSenderFromTheMailbox(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	mailbox, err := m.AddMailbox(domain.ID, "alerts", "secret-password-1", "Redock Alerts")
	if err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}
	// A local recipient, so the message lands on disk here and the header it
	// was actually sent with can be read back.
	if _, err := m.AddMailbox(domain.ID, "someone", "secret-password-2", "Someone"); err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}

	// A caller that only says what to send and to whom — which is how the
	// notification sender was written.
	msg := &EmailMessage{
		To:        []string{"someone@example.com"},
		Subject:   "alert",
		BodyPlain: "something needs attention",
	}
	if err := NewSMTPClient(m).SendEmail(mailbox.ID, msg); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if msg.From != "alerts@example.com" {
		t.Errorf("From = %q, want the mailbox address", msg.From)
	}
	if msg.FromName != "Redock Alerts" {
		t.Errorf("FromName = %q, want the mailbox name", msg.FromName)
	}

	// And the message that was actually delivered carries the header.
	account := m.LookupAccount("someone@example.com")
	if account == nil {
		t.Fatal("the recipient mailbox is missing")
	}
	messages, err := m.store().List(account.Base, inboxName)
	if err != nil || len(messages) != 1 {
		t.Fatalf("expected one delivered message, got %d (%v)", len(messages), err)
	}
	raw, err := m.store().Read(account.Base, inboxName, messages[0])
	if err != nil {
		t.Fatalf("read the message: %v", err)
	}
	from := headerValue(raw, "From")
	if !strings.Contains(from, "alerts@example.com") {
		t.Errorf("the delivered message has From: %q", from)
	}
}

// An explicit sender is left alone.
func TestSendEmailKeepsAnExplicitSender(t *testing.T) {
	m := newTestManager(t)
	domain, _ := m.AddDomain("example.com", "test")
	mailbox, err := m.AddMailbox(domain.ID, "alerts", "secret-password-1", "Redock Alerts")
	if err != nil {
		t.Fatalf("AddMailbox: %v", err)
	}

	msg := &EmailMessage{
		From: "alerts@example.com", FromName: "Chosen Name",
		To: []string{"someone@example.com"}, Subject: "alert", BodyPlain: "body",
	}
	if err := NewSMTPClient(m).SendEmail(mailbox.ID, msg); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if msg.FromName != "Chosen Name" {
		t.Errorf("FromName = %q, want the caller's choice", msg.FromName)
	}
}
