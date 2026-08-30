package email_server

import (
	"bufio"
	"fmt"
	"net"
	netsmtp "net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

// freePort asks the OS for an unused port, so the end-to-end test never
// collides with something already running on the machine.
func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a local port in this environment: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// startTestServer brings up the native engine on ephemeral ports with the
// network-dependent checks disabled.
func startTestServer(t *testing.T) (*EmailManager, map[string]int) {
	t.Helper()

	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	ports := map[string]int{
		"smtp":       freePort(t),
		"submission": freePort(t),
		"imap":       freePort(t),
		"pop3":       freePort(t),
	}

	m.config.SMTPPort = ports["smtp"]
	m.config.SubmissionPort = ports["submission"]
	m.config.IMAPPort = ports["imap"]
	m.config.POP3Port = ports["pop3"]
	m.config.IMAPEnabled = true
	m.config.POP3Enabled = true
	// Implicit-TLS listeners and inbound DNS checks are exercised elsewhere;
	// keeping them off here makes the test hermetic.
	m.config.SMTPSEnabled = false
	m.config.IMAPsEnabled = false
	m.config.POP3sEnabled = false
	m.config.STARTTLSRequired = false
	m.config.CheckSPF = false
	m.config.CheckDKIM = false
	m.config.CheckDMARC = false

	if err := m.Native().Start(); err != nil {
		t.Skipf("cannot start the native mail server here: %v", err)
	}
	t.Cleanup(func() { m.Native().Stop() })

	return m, ports
}

// deliverTestMessage pushes one message through the inbound SMTP listener.
func deliverTestMessage(t *testing.T, port int, from, to, subject string) {
	t.Helper()

	client, err := netsmtp.Dial(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial SMTP: %v", err)
	}
	defer client.Close()

	if err := client.Hello("test.local"); err != nil {
		t.Fatalf("HELO: %v", err)
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
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nHello from the test.\r\n", from, to, subject)
	if _, err := writer.Write([]byte(message)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	_ = client.Quit()
}

func TestEndToEndInboundDeliveryAndIMAPRetrieval(t *testing.T) {
	m, ports := startTestServer(t)

	deliverTestMessage(t, ports["smtp"], "sender@outside.test", "alice@example.com", "e2e subject")

	// The message must be on disk in the recipient's INBOX.
	account := m.LookupAccount("alice@example.com")
	messages, err := m.store().List(account.Base, inboxName)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 delivered message, got %d", len(messages))
	}

	raw, err := m.store().Read(account.Base, inboxName, messages[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(string(raw), "Subject: e2e subject") {
		t.Error("the delivered message lost its subject")
	}
	if !strings.Contains(string(raw), "Received: from") {
		t.Error("the server did not add a Received trace header")
	}

	// And it must be retrievable over IMAP.
	client, err := imapclient.Dial(fmt.Sprintf("127.0.0.1:%d", ports["imap"]))
	if err != nil {
		t.Fatalf("dial IMAP: %v", err)
	}
	defer client.Logout()

	if err := client.Login("alice@example.com", "secret"); err != nil {
		t.Fatalf("IMAP login: %v", err)
	}

	status, err := client.Select(inboxName, false)
	if err != nil {
		t.Fatalf("SELECT INBOX: %v", err)
	}
	if status.Messages != 1 {
		t.Fatalf("IMAP reports %d messages, expected 1", status.Messages)
	}
	if status.UidValidity == 0 {
		t.Error("UIDVALIDITY must be non-zero")
	}
}

func TestIMAPRejectsBadCredentials(t *testing.T) {
	_, ports := startTestServer(t)

	client, err := imapclient.Dial(fmt.Sprintf("127.0.0.1:%d", ports["imap"]))
	if err != nil {
		t.Fatalf("dial IMAP: %v", err)
	}
	defer client.Logout()

	if err := client.Login("alice@example.com", "wrong-password"); err == nil {
		t.Fatal("IMAP accepted a wrong password")
	}
}

func TestIMAPListsDefaultFolders(t *testing.T) {
	_, ports := startTestServer(t)

	client, err := imapclient.Dial(fmt.Sprintf("127.0.0.1:%d", ports["imap"]))
	if err != nil {
		t.Fatalf("dial IMAP: %v", err)
	}
	defer client.Logout()

	if err := client.Login("alice@example.com", "secret"); err != nil {
		t.Fatalf("IMAP login: %v", err)
	}

	mailboxes := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- client.List("", "*", mailboxes)
	}()

	names := map[string]bool{}
	for info := range mailboxes {
		names[info.Name] = true
	}
	if err := <-done; err != nil {
		t.Fatalf("LIST: %v", err)
	}

	for _, expected := range append([]string{inboxName}, DefaultFolders...) {
		if !names[expected] {
			t.Errorf("folder %s missing from the IMAP listing (%v)", expected, names)
		}
	}
}

func TestEndToEndPOP3Retrieval(t *testing.T) {
	_, ports := startTestServer(t)

	deliverTestMessage(t, ports["smtp"], "sender@outside.test", "alice@example.com", "pop3 subject")

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ports["pop3"]), 5*time.Second)
	if err != nil {
		t.Fatalf("dial POP3: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	expectOK := func(step string) string {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("%s: read failed: %v", step, err)
		}
		if !strings.HasPrefix(line, "+OK") {
			t.Fatalf("%s: server said %q", step, strings.TrimSpace(line))
		}
		return line
	}
	send := func(command string) {
		if _, err := fmt.Fprintf(conn, "%s\r\n", command); err != nil {
			t.Fatalf("send %q: %v", command, err)
		}
	}

	expectOK("greeting")

	send("USER alice@example.com")
	expectOK("USER")

	send("PASS secret")
	expectOK("PASS")

	send("STAT")
	stat := expectOK("STAT")
	if !strings.Contains(stat, "+OK 1 ") {
		t.Fatalf("STAT should report exactly one message, said %q", strings.TrimSpace(stat))
	}

	send("RETR 1")
	expectOK("RETR")

	body := strings.Builder{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("RETR body: %v", err)
		}
		if strings.TrimSpace(line) == "." {
			break
		}
		body.WriteString(line)
	}
	if !strings.Contains(body.String(), "Subject: pop3 subject") {
		t.Errorf("RETR did not return the message:\n%s", body.String())
	}

	send("QUIT")
	expectOK("QUIT")
}

func TestSubmissionRelaysThroughTheQueue(t *testing.T) {
	m, ports := startTestServer(t)

	client, err := netsmtp.Dial(fmt.Sprintf("127.0.0.1:%d", ports["submission"]))
	if err != nil {
		t.Fatalf("dial submission: %v", err)
	}
	defer client.Close()

	if err := client.Hello("test.local"); err != nil {
		t.Fatalf("HELO: %v", err)
	}

	auth := netsmtp.PlainAuth("", "alice@example.com", "secret", "127.0.0.1")
	if err := client.Auth(auth); err != nil {
		t.Fatalf("AUTH: %v", err)
	}

	if err := client.Mail("alice@example.com"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	// A remote recipient is accepted on submission and must end up queued
	// rather than delivered.
	if err := client.Rcpt("bob@far-away.test"); err != nil {
		t.Fatalf("RCPT TO: %v", err)
	}

	writer, err := client.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	if _, err := writer.Write([]byte("From: alice@example.com\r\nTo: bob@far-away.test\r\nSubject: queued\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	_ = client.Quit()

	items := m.QueueItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(items))
	}
	if items[0].From != "alice@example.com" || len(items[0].Recipients) != 1 || items[0].Recipients[0] != "bob@far-away.test" {
		t.Fatalf("queued item is wrong: %+v", items[0])
	}
	if items[0].Subject != "queued" {
		t.Errorf("queue metadata lost the subject: %+v", items[0])
	}

	// Deleting it through the admin API empties the queue.
	if err := m.DeleteQueueItem(items[0].ID); err != nil {
		t.Fatalf("DeleteQueueItem: %v", err)
	}
	if len(m.QueueItems()) != 0 {
		t.Error("the queue should be empty after deletion")
	}
}

func TestInboundRefusesUnknownRecipientOverTheWire(t *testing.T) {
	_, ports := startTestServer(t)

	client, err := netsmtp.Dial(fmt.Sprintf("127.0.0.1:%d", ports["smtp"]))
	if err != nil {
		t.Fatalf("dial SMTP: %v", err)
	}
	defer client.Close()

	if err := client.Hello("test.local"); err != nil {
		t.Fatalf("HELO: %v", err)
	}
	if err := client.Mail("spammer@outside.test"); err != nil {
		t.Fatalf("MAIL FROM: %v", err)
	}
	if err := client.Rcpt("victim@somewhere-else.test"); err == nil {
		t.Fatal("the server relayed for a foreign domain")
	}
}
