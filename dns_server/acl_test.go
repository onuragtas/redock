package dns_server

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestClientACL(t *testing.T) {
	def, errs := parseClientACL((&DNSConfig{}).GetAllowedClientsList())
	if len(errs) > 0 {
		t.Fatalf("default list has invalid entries: %v", errs)
	}
	custom, errs := parseClientACL([]string{"203.0.113.0/24", "198.51.100.7", " ", "not-an-ip", "10.0.0.0/99"})
	if len(errs) != 2 {
		t.Fatalf("want 2 invalid entries, got %v", errs)
	}

	cases := []struct {
		ip          string
		def, custom bool
	}{
		{"127.0.0.1", true, false},
		{"10.0.70.5", true, false},
		{"172.20.1.2", true, false},
		{"192.168.1.10", true, false},
		{"100.64.3.4", true, false},
		{"::1", true, false},
		{"fd00::1", true, false},
		{"65.21.212.14", false, false}, // a public address
		{"8.8.8.8", false, false},
		{"203.0.113.9", false, true},
		{"198.51.100.7", false, true},
		{"198.51.100.8", false, false},
		{"::ffff:10.1.2.3", true, false}, // IPv4-mapped IPv6 counts as IPv4
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if got := def.allows(ip); got != c.def {
			t.Errorf("default allows(%s) = %v, want %v", c.ip, got, c.def)
		}
		if got := custom.allows(ip); got != c.custom {
			t.Errorf("custom allows(%s) = %v, want %v", c.ip, got, c.custom)
		}
	}
	if def.allows(nil) {
		t.Error("an unknown source address must be refused")
	}
	open, _ := parseClientACL([]string{"0.0.0.0/0", "::/0"})
	if !open.allows(net.ParseIP("8.8.8.8")) || !open.allows(net.ParseIP("2001:db8::1")) {
		t.Error("0.0.0.0/0 + ::/0 must allow everyone")
	}
}

func TestValidateAllowedClients(t *testing.T) {
	if err := ValidateAllowedClients([]string{"10.0.0.0/8", "192.168.1.1"}); err != nil {
		t.Errorf("valid list rejected: %v", err)
	}
	if err := ValidateAllowedClients([]string{"10.0.0.0/8", "nope"}); err == nil {
		t.Error("invalid entry accepted")
	}
}

// fakeWriter records what the handler answers and pretends to be a client at addr.
type fakeWriter struct {
	dns.ResponseWriter
	addr net.Addr
	msg  *dns.Msg
}

func (w *fakeWriter) RemoteAddr() net.Addr      { return w.addr }
func (w *fakeWriter) WriteMsg(m *dns.Msg) error { w.msg = m; return nil }

func TestHandlerRefusesClientsOutsideTheList(t *testing.T) {
	s := &DNSServer{config: &DNSConfig{}}
	s.rebuildACL() // default: private networks only

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	w := &fakeWriter{addr: &net.UDPAddr{IP: net.ParseIP("8.8.4.4"), Port: 5353}}
	s.handleDNSRequest(w, q)
	if w.msg == nil || w.msg.Rcode != dns.RcodeRefused {
		t.Fatalf("public client: want REFUSED, got %+v", w.msg)
	}
	if w.msg.RecursionAvailable {
		t.Error("a refused answer must not advertise recursion")
	}
	if len(w.msg.Answer) != 0 {
		t.Error("a refused answer must carry no records")
	}
}
