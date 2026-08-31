package email_server

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestDNSBLQueryNameReversesTheAddress(t *testing.T) {
	tests := []struct {
		ip   string
		zone string
		want string
	}{
		// The convention every list uses: octets reversed, zone appended.
		{"203.0.113.5", "zen.spamhaus.org", "5.113.0.203.zen.spamhaus.org"},
		{"1.2.3.4", "bl.example.com", "4.3.2.1.bl.example.com"},
		// IPv6 goes nibble by nibble, lowest first.
		{"2001:db8::1", "bl.example.com",
			"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.bl.example.com"},
	}

	for _, tc := range tests {
		got := dnsblQueryName(net.ParseIP(tc.ip), tc.zone)
		if got != tc.want {
			t.Errorf("dnsblQueryName(%s, %s) = %q, want %q", tc.ip, tc.zone, got, tc.want)
		}
	}
}

// Asking a public list about a private address gives away the shape of the
// local network and cannot come back listed, so it is never asked.
func TestDNSBLSkipsAddressesThatCannotBeListed(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.70.5", "192.168.1.20", "172.16.4.1", "::1", "169.254.1.1"} {
		if dnsblCheckable(net.ParseIP(address)) {
			t.Errorf("%s should not be sent to a block list", address)
		}
	}
	for _, address := range []string{"203.0.113.5", "8.8.8.8", "2001:db8::1"} {
		if !dnsblCheckable(net.ParseIP(address)) {
			t.Errorf("%s is a public address and should be checked", address)
		}
	}
	if dnsblCheckable(nil) {
		t.Error("a missing address should not be checked")
	}
}

func TestParseDNSBLZonesAcceptsEitherSeparator(t *testing.T) {
	zones := parseDNSBLZones("zen.spamhaus.org, bl.spamcop.net\n  dnsbl.example.com. ;")
	want := []string{"zen.spamhaus.org", "bl.spamcop.net", "dnsbl.example.com"}
	if strings.Join(zones, "|") != strings.Join(want, "|") {
		t.Errorf("parsed %v, want %v", zones, want)
	}

	// An empty setting still has to produce something to query, or turning the
	// check on would silently do nothing.
	if len(parseDNSBLZones("   ")) == 0 {
		t.Error("an empty zone list should fall back to the defaults")
	}
}

// The check must never be a reason mail fails to arrive: a list that cannot be
// reached is treated as saying nothing.
func TestDNSBLTreatsAnUnreachableListAsNotListed(t *testing.T) {
	// A zone under .invalid can never resolve (RFC 2606).
	result := queryDNSBLZones(context.Background(), net.DefaultResolver,
		net.ParseIP("203.0.113.5"), []string{"list.invalid"})

	if result.Listed {
		t.Errorf("an unreachable list reported a listing: %+v", result)
	}
}

func TestDNSBLIsOffUntilTurnedOn(t *testing.T) {
	m := newTestManager(t)

	cfg := m.nativeConfig()
	if cfg.DNSBLEnabled {
		t.Error("block-list checking should be off until the operator enables it")
	}

	// Off means no lookup at all, whatever the address.
	if result := m.checkDNSBL(net.ParseIP("203.0.113.5"), cfg); result.Listed {
		t.Errorf("a disabled check reported a listing: %+v", result)
	}
}

func TestDNSBLCacheStaysBounded(t *testing.T) {
	cache := &dnsblCache{entries: make(map[string]dnsblCacheEntry)}
	for i := 0; i < dnsblCacheMax*2; i++ {
		cache.put(net.IPv4(203, 0, byte(i/256), byte(i%256)).String(), DNSBLResult{Listed: true})
	}
	if len(cache.entries) > dnsblCacheMax {
		t.Errorf("cache grew to %d entries, ceiling is %d", len(cache.entries), dnsblCacheMax)
	}

	if dropped := cache.clear(); dropped == 0 {
		t.Error("clear reported nothing to forget")
	}
}
