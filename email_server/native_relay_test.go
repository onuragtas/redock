package email_server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// relaySession plays a conversation by hand and returns the final reply to each
// command, which is what a prober actually sees.
func relaySession(t *testing.T, port int, steps ...string) []string {
	t.Helper()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)
	readReply := func() string {
		last := ""
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return last
			}
			last = strings.TrimRight(line, "\r\n")
			if len(last) < 4 || last[3] != '-' {
				return last
			}
		}
	}

	readReply() // banner
	replies := make([]string, 0, len(steps))
	for _, step := range steps {
		if _, err := fmt.Fprintf(conn, "%s\r\n", step); err != nil {
			t.Fatalf("write %q: %v", step, err)
		}
		replies = append(replies, readReply())
	}
	return replies
}

func startRelayTestServer(t *testing.T) (*EmailManager, map[string]int) {
	t.Helper()

	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	ports := map[string]int{"smtp": freePort(t), "submission": freePort(t)}
	m.config.SMTPPort = ports["smtp"]
	m.config.SubmissionPort = ports["submission"]
	m.config.SMTPSEnabled = false
	m.config.IMAPEnabled = false
	m.config.IMAPsEnabled = false
	m.config.POP3Enabled = false
	m.config.POP3sEnabled = false
	m.config.STARTTLSRequired = false
	m.config.CheckSPF = false
	m.config.CheckDKIM = false
	m.config.CheckDMARC = false
	// A test manager starts from a bare config; the guard is what these tests
	// are about, so turn it on the way a real install does.
	m.config.GuardEnabled = true

	if err := m.Native().Start(); err != nil {
		t.Skipf("cannot start the native mail server here: %v", err)
	}
	t.Cleanup(func() { m.Native().Stop() })
	return m, ports
}

// The probe worth guarding against is not "mail from a stranger" but "mail
// through us to a stranger". A server that carries it is an open relay and its
// address is on a block list within hours.
//
// The sender claiming to belong to a domain we host must not change that. Some
// servers trust a local-looking MAIL FROM; the decision belongs to the
// recipient, which is the only address that says where the mail is going.
func TestRelayIsRefusedEvenWhenTheSenderLooksLocal(t *testing.T) {
	_, ports := startRelayTestServer(t)

	replies := relaySession(t, ports["smtp"],
		"EHLO prober.test",
		"MAIL FROM:<alice@example.com>", // a domain this server hosts
		"RCPT TO:<someone@gmail.com>",   // a domain it does not
		"QUIT",
	)

	if !strings.HasPrefix(replies[1], "250") {
		t.Errorf("MAIL FROM = %q, want it accepted: the decision belongs to RCPT", replies[1])
	}
	if !strings.HasPrefix(replies[2], "550") {
		t.Fatalf("RCPT TO = %q, want a 550 refusal — this server would be an open relay", replies[2])
	}
}

// Ordinary inbound mail still has to work: the same sender, addressed to a
// mailbox this server actually hosts.
func TestMailToALocalRecipientIsStillAccepted(t *testing.T) {
	_, ports := startRelayTestServer(t)

	replies := relaySession(t, ports["smtp"],
		"EHLO sender.test",
		"MAIL FROM:<someone@outside.test>",
		"RCPT TO:<alice@example.com>",
		"QUIT",
	)

	if !strings.HasPrefix(replies[2], "250") {
		t.Errorf("RCPT TO for a local mailbox = %q, want it accepted", replies[2])
	}
}

// The submission port refuses before the envelope is even read.
func TestSubmissionRefusesBeforeTheEnvelopeWithoutAuth(t *testing.T) {
	_, ports := startRelayTestServer(t)

	replies := relaySession(t, ports["submission"],
		"EHLO prober.test",
		"MAIL FROM:<alice@example.com>",
		"QUIT",
	)

	if !strings.HasPrefix(replies[1], "502") && !strings.HasPrefix(replies[1], "530") {
		t.Errorf("MAIL FROM without auth = %q, want it refused", replies[1])
	}
}

// Refusing costs the prober nothing unless the attempt is remembered. Three
// tries and the address stops being answered at all.
func TestRepeatedRelayAttemptsEarnABlock(t *testing.T) {
	m, ports := startRelayTestServer(t)

	// The guard exempts private ranges, and a test connects over loopback, so
	// point it at a public address for the duration.
	guard := m.guard()
	guard.mu.Lock()
	guard.allowList = nil
	guard.mu.Unlock()

	cfg := m.nativeConfig()
	if cfg.MaxRelayAttempts != 3 {
		t.Fatalf("MaxRelayAttempts = %d, want the default of 3", cfg.MaxRelayAttempts)
	}

	for i := 0; i < cfg.MaxRelayAttempts; i++ {
		relaySession(t, ports["smtp"],
			"EHLO prober.test",
			"MAIL FROM:<alice@example.com>",
			"RCPT TO:<someone@gmail.com>",
			"QUIT",
		)
	}

	blocked := m.BlockedClients()
	if len(blocked) == 0 {
		t.Fatal("after repeated relay attempts the address is still welcome")
	}
	if !strings.Contains(blocked[0].Reason, "relay") {
		t.Errorf("blocked for %q, want the reason to name the relay attempts", blocked[0].Reason)
	}

	// And the block is real: the next connection is closed without a banner.
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ports["smtp"]))
	if err == nil {
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := bufio.NewReader(conn).ReadString('\n'); err == nil {
			t.Error("a blocked address was still greeted")
		}
	}
}

// A user's own client authenticates and then sends wherever it likes; that must
// never look like a relay attempt.
func TestAuthenticatedSubmissionIsNotCountedAsRelay(t *testing.T) {
	m, ports := startRelayTestServer(t)

	guard := m.guard()
	guard.mu.Lock()
	guard.allowList = nil
	guard.mu.Unlock()

	for i := 0; i < 5; i++ {
		submitTestMessage(t, ports["submission"], "alice@example.com", "someone@far-away.test",
			"hello", fmt.Sprintf("msg-%d@example.com", i))
	}

	if blocked := m.BlockedClients(); len(blocked) != 0 {
		t.Errorf("sending authenticated mail got the client blocked: %+v", blocked)
	}
}
