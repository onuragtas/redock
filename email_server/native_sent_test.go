package email_server

import (
	"fmt"
	netsmtp "net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

// submitTestMessage sends one authenticated message through the submission
// listener, the way any script or application that "just sends over SMTP" does.
func submitTestMessage(t *testing.T, port int, from, to, subject, messageID string) {
	t.Helper()

	client, err := netsmtp.Dial(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial submission: %v", err)
	}
	defer client.Close()

	if err := client.Hello("test.local"); err != nil {
		t.Fatalf("HELO: %v", err)
	}
	if err := client.Auth(netsmtp.PlainAuth("", from, "secret", "127.0.0.1")); err != nil {
		t.Fatalf("AUTH: %v", err)
	}
	if err := client.Mail(from); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := client.Rcpt(to); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}

	writer, err := client.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\n\r\nbody\r\n",
		from, to, subject, messageID)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	_ = client.Quit()
}

// Mail sent over SMTP used to leave nothing behind for the sender: it was
// delivered or queued, logged, and never written to their own mailbox. The
// dashboard's Sent folder stayed empty and the message looked lost.
func TestSubmittedMailIsFiledInSent(t *testing.T) {
	m, ports := startTestServer(t)

	submitTestMessage(t, ports["submission"], "alice@example.com", "bob@far-away.test", "kept", "kept-1@example.com")

	account := m.LookupAccount("alice@example.com")
	messages, err := m.store().List(account.Base, sentName)
	if err != nil {
		t.Fatalf("list Sent: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Sent holds %d messages, want 1", len(messages))
	}

	// Sent mail has been read by definition, so it must not show as unread.
	if !hasFlag(messages[0].Flags, imapFlagSeen) {
		t.Errorf("the Sent copy is marked unread: %v", messages[0].Flags)
	}

	// And it has to be visible through the webmail API, which is where the
	// gap was reported.
	emails, err := m.WebmailMessages(account.Mailbox.ID, sentName, 50)
	if err != nil {
		t.Fatalf("WebmailMessages: %v", err)
	}
	if len(emails) != 1 {
		t.Fatalf("webmail shows %d messages in Sent, want 1", len(emails))
	}
	if emails[0].Subject != "kept" {
		t.Errorf("Subject = %q, want %q", emails[0].Subject, "kept")
	}
}

// A desktop client saves its own copy over IMAP after submitting. Now that the
// server files one too, the two must not both land in the folder.
func TestClientAppendDoesNotDuplicateTheSentCopy(t *testing.T) {
	m, ports := startTestServer(t)

	const messageID = "dup-1@example.com"
	submitTestMessage(t, ports["submission"], "alice@example.com", "bob@far-away.test", "once", messageID)

	client, err := imapclient.Dial(fmt.Sprintf("127.0.0.1:%d", ports["imap"]))
	if err != nil {
		t.Fatalf("dial IMAP: %v", err)
	}
	defer client.Logout()

	if err := client.Login("alice@example.com", "secret"); err != nil {
		t.Fatalf("LOGIN: %v", err)
	}

	appended := fmt.Sprintf("From: alice@example.com\r\nTo: bob@far-away.test\r\nSubject: once\r\nMessage-ID: <%s>\r\n\r\nbody\r\n", messageID)
	if err := client.Append(sentName, []string{imap.SeenFlag}, time.Now(), strings.NewReader(appended)); err != nil {
		t.Fatalf("APPEND: %v", err)
	}

	account := m.LookupAccount("alice@example.com")
	messages, err := m.store().List(account.Base, sentName)
	if err != nil {
		t.Fatalf("list Sent: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Sent holds %d copies of the same message, want 1", len(messages))
	}
}

// Suppressing duplicates must not swallow a genuinely different message.
func TestAppendOfADifferentMessageIsKept(t *testing.T) {
	m, ports := startTestServer(t)

	submitTestMessage(t, ports["submission"], "alice@example.com", "bob@far-away.test", "first", "first-1@example.com")

	client, err := imapclient.Dial(fmt.Sprintf("127.0.0.1:%d", ports["imap"]))
	if err != nil {
		t.Fatalf("dial IMAP: %v", err)
	}
	defer client.Logout()

	if err := client.Login("alice@example.com", "secret"); err != nil {
		t.Fatalf("LOGIN: %v", err)
	}

	appended := "From: alice@example.com\r\nTo: bob@far-away.test\r\nSubject: second\r\nMessage-ID: <second-1@example.com>\r\n\r\nbody\r\n"
	if err := client.Append(sentName, []string{imap.SeenFlag}, time.Now(), strings.NewReader(appended)); err != nil {
		t.Fatalf("APPEND: %v", err)
	}

	account := m.LookupAccount("alice@example.com")
	messages, err := m.store().List(account.Base, sentName)
	if err != nil {
		t.Fatalf("list Sent: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("Sent holds %d messages, want 2", len(messages))
	}
}

func TestMessageIDOfStripsAngleBrackets(t *testing.T) {
	raw := []byte("From: a@example.com\r\nMessage-ID: <abc@example.com>\r\n\r\nbody\r\n")
	if got := messageIDOf(raw); got != "abc@example.com" {
		t.Errorf("messageIDOf = %q, want %q", got, "abc@example.com")
	}
	if got := messageIDOf([]byte("From: a@example.com\r\n\r\nbody\r\n")); got != "" {
		t.Errorf("a message with no Message-ID gave %q, want empty", got)
	}
}
