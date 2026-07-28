//go:build darwin

package traffic_inspector

import (
	"net"
	"testing"
)

// fakeAddr lets us simulate an address whose String() representation
// differs in formatting from a plain *net.UDPAddr's, the way quic-go's own
// internal net.Addr wrapper can — this is exactly the mismatch that made
// OriginalDestinationFor's cache lookups silently miss cached entries.
type fakeAddr string

func (f fakeAddr) Network() string { return "udp" }
func (f fakeAddr) String() string  { return string(f) }

func TestCanonicalUDPAddrKey_NormalizesAcrossAddrTypes(t *testing.T) {
	udpAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 58418}

	// A differently-typed net.Addr for the exact same peer, formatted
	// slightly differently (as quic-go's own address representation might).
	other := fakeAddr("10.0.0.2:58418")

	key1 := canonicalUDPAddrKey(udpAddr)
	key2 := canonicalUDPAddrKey(other)

	if key1 != key2 {
		t.Fatalf("expected matching canonical keys for the same peer, got %q vs %q", key1, key2)
	}
	if key1 != "10.0.0.2:58418" {
		t.Fatalf("unexpected canonical key: %q", key1)
	}
}

func TestCanonicalUDPAddrKey_IPv4MappedIPv6(t *testing.T) {
	// Simulates an address that round-tripped through an IPv6-capable
	// representation and came back as an IPv4-mapped-IPv6 string.
	mapped := fakeAddr("::ffff:10.0.0.2:58418")
	plain := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 58418}

	// net.SplitHostPort can't parse "::ffff:10.0.0.2:58418" directly (it's
	// ambiguous without brackets), so exercise the realistic bracketed form
	// instead — this is how such an address actually stringifies via Go's
	// net.Addr conventions.
	mapped = fakeAddr("[::ffff:10.0.0.2]:58418")

	key1 := canonicalUDPAddrKey(mapped)
	key2 := canonicalUDPAddrKey(plain)

	if key1 != key2 {
		t.Fatalf("expected IPv4-mapped-IPv6 and plain IPv4 to normalize to the same key, got %q vs %q", key1, key2)
	}
}
