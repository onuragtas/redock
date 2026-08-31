package email_server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeMTA is a scriptable receiving server: it lets a test decide exactly what
// the far side answers, which is the only way to prove the delivery path keeps
// the remote's own words.
type fakeMTA struct {
	listener net.Listener
	// replies maps a command prefix to the response line to send.
	replies map[string]string
}

func newFakeMTA(t *testing.T, replies map[string]string) *fakeMTA {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen locally: %v", err)
	}

	mta := &fakeMTA{listener: listener, replies: replies}
	go mta.serve()
	t.Cleanup(func() { listener.Close() })
	return mta
}

func (f *fakeMTA) addr() string {
	host, port, _ := net.SplitHostPort(f.listener.Addr().String())
	_ = host
	return port
}

func (f *fakeMTA) reply(command string) string {
	for prefix, response := range f.replies {
		if strings.HasPrefix(strings.ToUpper(command), prefix) {
			return response
		}
	}
	return "250 OK"
}

func (f *fakeMTA) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			reader := bufio.NewReader(c)
			fmt.Fprintf(c, "%s\r\n", f.reply("GREETING"))

			inData := false
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimSpace(line)

				if inData {
					if trimmed == "." {
						inData = false
						fmt.Fprintf(c, "%s\r\n", f.reply("BODY"))
					}
					continue
				}

				upper := strings.ToUpper(trimmed)
				switch {
				case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
					fmt.Fprintf(c, "250 hello\r\n")
				case strings.HasPrefix(upper, "DATA"):
					fmt.Fprintf(c, "%s\r\n", f.reply("DATA"))
					if strings.HasPrefix(f.reply("DATA"), "354") {
						inData = true
					}
				case strings.HasPrefix(upper, "QUIT"):
					fmt.Fprintf(c, "221 bye\r\n")
					return
				default:
					fmt.Fprintf(c, "%s\r\n", f.reply(upper))
				}
			}
		}(conn)
	}
}

// deliverToFake runs a transaction against the fake server on its own port.
func deliverToFake(t *testing.T, mta *fakeMTA, from string, recipients []string) (*DeliveryResult, error) {
	t.Helper()

	// deliverSMTP always dials port 25, so the test drives the session pieces
	// through the same code path by pointing at the loopback listener.
	conn, err := net.DialTimeout("tcp", mta.listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	return deliverOverConn(conn, "127.0.0.1", "test.local", from, recipients,
		[]byte("Subject: probe\r\n\r\nbody\r\n"))
}

func TestDeliveryKeepsTheAcceptingReply(t *testing.T) {
	mta := newFakeMTA(t, map[string]string{
		"GREETING": "220 mx.example.com ESMTP",
		"MAIL":     "250 sender ok",
		"RCPT":     "250 recipient ok",
		"DATA":     "354 go ahead",
		"BODY":     "250 2.0.0 OK 1234567890 abc123si.42 - gsmtp",
	})

	result, err := deliverToFake(t, mta, "alice@example.com", []string{"bob@example.net"})
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	if result.Accepted.Code != 250 {
		t.Errorf("expected a 250, got %d", result.Accepted.Code)
	}
	// The remote queue id is the part support teams ask for.
	if !strings.Contains(result.Accepted.Text, "abc123si.42") {
		t.Errorf("the accepting reply text was lost: %q", result.Accepted.Text)
	}
	if len(result.Recipients) != 1 {
		t.Errorf("unexpected recipients: %v", result.Recipients)
	}
}

func TestDeliveryReportsATemporaryRefusal(t *testing.T) {
	mta := newFakeMTA(t, map[string]string{
		"GREETING": "220 mx.example.com ESMTP",
		"MAIL":     "421 4.7.0 Try again later",
	})

	_, err := deliverToFake(t, mta, "alice@example.com", []string{"bob@example.net"})
	if err == nil {
		t.Fatal("a 421 must surface as an error")
	}

	reply := replyOf(err)
	if reply.Code != 421 {
		t.Fatalf("the response code was lost: %+v", reply)
	}
	if !reply.Temporary() {
		t.Error("421 must be classified as temporary so the message is retried")
	}
	if isPermanentSMTPError(err) {
		t.Error("a 4xx must not be treated as a bounce")
	}
	if !strings.Contains(err.Error(), "Try again later") {
		t.Errorf("the remote's words were lost: %v", err)
	}
}

func TestDeliveryReportsAPermanentRefusal(t *testing.T) {
	mta := newFakeMTA(t, map[string]string{
		"GREETING": "220 mx.example.com ESMTP",
		"MAIL":     "250 ok",
		"RCPT":     "550 5.7.1 Message rejected due to sender reputation",
	})

	_, err := deliverToFake(t, mta, "alice@example.com", []string{"bob@example.net"})
	if err == nil {
		t.Fatal("a 550 must surface as an error")
	}

	reply := replyOf(err)
	if reply.Code != 550 {
		t.Fatalf("the response code was lost: %+v", reply)
	}
	if !isPermanentSMTPError(err) {
		t.Error("a 5xx must bounce rather than be retried forever")
	}
	if !strings.Contains(err.Error(), "sender reputation") {
		t.Errorf("the reason was lost: %v", err)
	}
}

func TestSMTPReplyClassification(t *testing.T) {
	cases := []struct {
		code                 int
		temporary, permanent bool
	}{
		{250, false, false},
		{421, true, false},
		{451, true, false},
		{550, false, true},
		{554, false, true},
	}
	for _, tc := range cases {
		reply := SMTPReply{Code: tc.code}
		if reply.Temporary() != tc.temporary || reply.Permanent() != tc.permanent {
			t.Errorf("%d classified wrongly: temporary=%v permanent=%v", tc.code, reply.Temporary(), reply.Permanent())
		}
	}
}
