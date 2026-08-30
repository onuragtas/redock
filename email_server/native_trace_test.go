package email_server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	netsmtp "net/smtp"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeLineHidesCredentials(t *testing.T) {
	cases := map[string]string{
		"AUTH PLAIN AGFsaWNlAHNlY3JldA==": "AUTH PLAIN [credentials hidden]",
		"auth plain":                      "AUTH plain [credentials hidden]",
		"PASS hunter2":                    "PASS [hidden]",
		"LOGIN alice secret":              "LOGIN [credentials hidden]",
		"EHLO client.test":                "EHLO client.test",
	}
	for input, want := range cases {
		if got := sanitizeLine(input); got != want {
			t.Errorf("sanitizeLine(%q) = %q, want %q", input, got, want)
		}
	}

	// The credential line that follows an AUTH command must never be recorded.
	conn := &tracedConn{}
	if got := conn.redact("in", "AUTH PLAIN"); got != "AUTH PLAIN" {
		t.Errorf("the AUTH command itself should be kept: %q", got)
	}
	if got := conn.redact("in", "AGFsaWNlAHNlY3JldA=="); got != "[credentials hidden]" {
		t.Errorf("the credential line leaked into the trace: %q", got)
	}
	if got := conn.redact("in", "MAIL FROM:<a@b.test>"); got != "MAIL FROM:<a@b.test>" {
		t.Errorf("only the credential line should be hidden: %q", got)
	}

	// A 334 challenge from the server has the same effect.
	conn2 := &tracedConn{}
	conn2.redact("out", "334 UGFzc3dvcmQ6")
	if got := conn2.redact("in", "c2VjcmV0"); got != "[credentials hidden]" {
		t.Errorf("the challenge response leaked into the trace: %q", got)
	}

	// Control characters must not reach the dashboard verbatim.
	if got := sanitizeLine("EHLO \x00\x07bad"); strings.ContainsAny(got, "\x00\x07") {
		t.Errorf("unprintable bytes survived sanitising: %q", got)
	}
}

func TestTraceStoreKeepsNewestAndBounds(t *testing.T) {
	store := newTraceStore()

	for i := 1; i <= maxTracedConnections+5; i++ {
		store.begin(&ConnectionTrace{ID: uint64(i), StartedAt: time.Now(), Service: "smtp"})
		store.end(uint64(i), "")
	}

	traces := store.List(0)
	if len(traces) != maxTracedConnections {
		t.Fatalf("expected the ring to hold %d traces, got %d", maxTracedConnections, len(traces))
	}
	if traces[0].ID != uint64(maxTracedConnections+5) {
		t.Fatalf("expected the newest connection first, got %d", traces[0].ID)
	}
}

func TestTraceStoreCapsLinesPerConnection(t *testing.T) {
	store := newTraceStore()
	store.begin(&ConnectionTrace{ID: 1, StartedAt: time.Now()})

	for i := 0; i < maxTraceLines+50; i++ {
		store.append(1, "in", fmt.Sprintf("LINE %d", i))
	}

	traces := store.List(1)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if len(traces[0].Lines) != maxTraceLines {
		t.Fatalf("expected the trace to stop at %d lines, got %d", maxTraceLines, len(traces[0].Lines))
	}
	if !traces[0].Truncated {
		t.Error("an over-long trace must be marked truncated")
	}
}

// TestConnectionTraceRecordsRefusedTLSHandshake reproduces the reported case: a
// client that starts STARTTLS and then rejects the certificate. Nothing is
// delivered, so the message log stays empty — the connection trace is what has
// to explain it.
func TestConnectionTraceRecordsRefusedTLSHandshake(t *testing.T) {
	m, ports := startTestServer(t)

	client, err := netsmtp.Dial(fmt.Sprintf("127.0.0.1:%d", ports["smtp"]))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := client.Hello("picky-client.test"); err != nil {
		t.Fatalf("HELO: %v", err)
	}

	// Verify against an empty pool so our self-signed certificate is refused,
	// exactly as another server would refuse it.
	err = client.StartTLS(&tls.Config{ServerName: "mail.example.com", RootCAs: x509.NewCertPool()})
	if err == nil {
		t.Fatal("the handshake was expected to fail verification")
	}
	client.Close()

	// Give the server a moment to finish closing the connection.
	deadline := time.Now().Add(3 * time.Second)
	var trace *ConnectionTrace
	for time.Now().Before(deadline) {
		for _, candidate := range m.ConnectionTraces(50) {
			if candidate.Service != "smtp" {
				continue
			}
			for _, line := range candidate.Lines {
				if strings.Contains(line.Text, "picky-client.test") {
					c := candidate
					trace = &c
					break
				}
			}
			if trace != nil {
				break
			}
		}
		if trace != nil && trace.EndedAt != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if trace == nil {
		t.Fatal("the connection was not recorded at all — this is the gap being fixed")
	}
	if trace.RemoteIP == "" {
		t.Error("the trace has no remote address")
	}

	joined := ""
	for _, line := range trace.Lines {
		joined += line.Direction + " " + line.Text + "\n"
	}
	if !strings.Contains(joined, "EHLO picky-client.test") {
		t.Errorf("the client's commands were not recorded:\n%s", joined)
	}
	if !strings.Contains(joined, "STARTTLS") {
		t.Errorf("the STARTTLS attempt was not recorded:\n%s", joined)
	}
	if !trace.Encrypted {
		t.Errorf("the trace should note that the handshake began:\n%s", joined)
	}
}

func TestConnectionTraceRecordsAPlainSession(t *testing.T) {
	m, ports := startTestServer(t)

	deliverTestMessage(t, ports["smtp"], "sender@outside.test", "alice@example.com", "traced")

	var trace *ConnectionTrace
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && trace == nil {
		for _, candidate := range m.ConnectionTraces(50) {
			for _, line := range candidate.Lines {
				if strings.Contains(line.Text, "sender@outside.test") {
					c := candidate
					trace = &c
					break
				}
			}
			if trace != nil {
				break
			}
		}
		if trace == nil {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if trace == nil {
		t.Fatal("a completed delivery left no connection trace")
	}

	joined := ""
	for _, line := range trace.Lines {
		joined += line.Text + "\n"
	}
	for _, want := range []string{"MAIL FROM", "RCPT TO", "DATA"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%s missing from the trace:\n%s", want, joined)
		}
	}
}

func TestConnectEventsReachTheMessageLog(t *testing.T) {
	m, ports := startTestServer(t)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ports["smtp"]), 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()

	deadline := time.Now().Add(3 * time.Second)
	var sawConnect, sawDisconnect bool
	for time.Now().Before(deadline) && !(sawConnect && sawDisconnect) {
		result, err := m.GetMailLogs(MailLogQuery{Direction: "system"})
		if err != nil {
			t.Fatalf("GetMailLogs: %v", err)
		}
		for _, entry := range result.Entries {
			switch entry.Status {
			case "connect":
				sawConnect = true
			case "disconnect", "conn-error":
				sawDisconnect = true
			}
		}
		if !(sawConnect && sawDisconnect) {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if !sawConnect {
		t.Error("opening a connection produced no log entry")
	}
	if !sawDisconnect {
		t.Error("closing a connection produced no log entry")
	}
}

func TestSelfSignedCertificateCoversLocalAddresses(t *testing.T) {
	dir := t.TempDir()
	ips := append(localAddresses(), net.ParseIP("203.0.113.5"))
	manager := newCertManager("mail.example.com", dir, filepath.Join(dir, "work"), "", "",
		[]string{"mail.example.com", "mail.other.test"}, ips)

	pair, err := manager.selfSigned()
	if err != nil {
		t.Fatalf("selfSigned: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// The reported failure was "doesn't contain any IP SANs".
	if len(leaf.IPAddresses) == 0 {
		t.Fatal("the certificate carries no IP SANs")
	}
	if err := leaf.VerifyHostname("203.0.113.5"); err != nil {
		t.Errorf("the certificate does not cover the address a client dialled: %v", err)
	}
	for _, name := range []string{"mail.example.com", "mail.other.test", "localhost"} {
		if err := leaf.VerifyHostname(name); err != nil {
			t.Errorf("the certificate does not cover %s: %v", name, err)
		}
	}

	// A second call must reuse the file rather than issue a new certificate.
	again, err := manager.selfSigned()
	if err != nil {
		t.Fatalf("selfSigned (second call): %v", err)
	}
	againLeaf, _ := x509.ParseCertificate(again.Certificate[0])
	if againLeaf.SerialNumber.Cmp(leaf.SerialNumber) != 0 {
		t.Error("an unchanged identity set must not re-issue the certificate")
	}
}

func TestCertificateIsReissuedWhenAnAddressAppears(t *testing.T) {
	dir := t.TempDir()

	first := newCertManager("mail.example.com", dir, filepath.Join(dir, "work"), "", "", nil, nil)
	pair, err := first.selfSigned()
	if err != nil {
		t.Fatalf("selfSigned: %v", err)
	}
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])

	// The same directory, but now a new address must be covered.
	second := newCertManager("mail.example.com", dir, filepath.Join(dir, "work"), "", "",
		nil, []net.IP{net.ParseIP("203.0.113.9")})
	updated, err := second.selfSigned()
	if err != nil {
		t.Fatalf("selfSigned (after new address): %v", err)
	}
	updatedLeaf, _ := x509.ParseCertificate(updated.Certificate[0])

	if updatedLeaf.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
		t.Fatal("a new address must trigger a re-issue")
	}
	if err := updatedLeaf.VerifyHostname("203.0.113.9"); err != nil {
		t.Errorf("the re-issued certificate does not cover the new address: %v", err)
	}
}
