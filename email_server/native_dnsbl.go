package email_server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// A DNS block list answers one question about a connecting address: has this
// machine been sending spam elsewhere? It is the cheapest filter a mail server
// has, because the answer arrives before the message body does — no content to
// parse, no model to train.
//
// The mechanics are a DNS convention rather than a protocol: reverse the
// address, append the list's zone, and ask for an A record. An answer means
// listed, NXDOMAIN means not, and the matching TXT record carries the reason.

const (
	// defaultDNSBLZones is what the settings start with. Nothing is queried
	// until the operator turns the check on.
	defaultDNSBLZones = "zen.spamhaus.org, bl.spamcop.net"
	// dnsblTimeout bounds the whole lookup. A block list that is slow must not
	// hold up mail: an unanswered query is treated as "not listed".
	dnsblTimeout = 3 * time.Second
	// dnsblCacheTTL is how long an answer is reused. A sender opens several
	// connections in a row, and asking a public list once per connection is
	// both slow and rude.
	dnsblCacheTTL = 10 * time.Minute
	// dnsblCacheMax bounds the cache; past it the oldest answers are forgotten.
	dnsblCacheMax = 2048
)

// DNSBLResult is what the lists said about one address.
type DNSBLResult struct {
	Listed bool   `json:"listed"`
	Zone   string `json:"zone,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// dnsblCache remembers recent answers.
type dnsblCache struct {
	mu      sync.Mutex
	entries map[string]dnsblCacheEntry
}

type dnsblCacheEntry struct {
	result  DNSBLResult
	expires time.Time
}

var dnsblAnswers = &dnsblCache{entries: make(map[string]dnsblCacheEntry)}

func (c *dnsblCache) get(key string) (DNSBLResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expires) {
		return DNSBLResult{}, false
	}
	return entry.result, true
}

func (c *dnsblCache) put(key string, result DNSBLResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= dnsblCacheMax {
		now := time.Now()
		for k, entry := range c.entries {
			if now.After(entry.expires) {
				delete(c.entries, k)
			}
		}
		// Expiry alone is not a bound when every answer is fresh.
		for k := range c.entries {
			if len(c.entries) < dnsblCacheMax {
				break
			}
			delete(c.entries, k)
		}
	}
	c.entries[key] = dnsblCacheEntry{result: result, expires: time.Now().Add(dnsblCacheTTL)}
}

// clear empties the cache and reports how many answers were forgotten.
func (c *dnsblCache) clear() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	dropped := len(c.entries)
	if dropped == 0 {
		return 0
	}
	c.entries = make(map[string]dnsblCacheEntry)
	return dropped
}

// parseDNSBLZones splits the configured list, accepting commas or newlines.
func parseDNSBLZones(configured string) []string {
	if strings.TrimSpace(configured) == "" {
		configured = defaultDNSBLZones
	}

	zones := make([]string, 0, 4)
	for _, zone := range strings.FieldsFunc(configured, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';'
	}) {
		zone = strings.TrimSuffix(strings.TrimSpace(zone), ".")
		if zone != "" {
			zones = append(zones, zone)
		}
	}
	return zones
}

// dnsblQueryName builds the name to look up: the address reversed, then the
// list's zone. IPv4 reverses its octets; IPv6 reverses its nibbles.
func dnsblQueryName(ip net.IP, zone string) string {
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.%s", ip4[3], ip4[2], ip4[1], ip4[0], zone)
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return ""
	}
	var b strings.Builder
	for i := len(ip16) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "%x.%x.", ip16[i]&0x0f, ip16[i]>>4)
	}
	b.WriteString(zone)
	return b.String()
}

// dnsblCheckable reports whether an address is worth asking about. A private or
// loopback address cannot be listed, and sending it to a public resolver would
// leak the shape of the local network for no answer.
func dnsblCheckable(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// checkDNSBL asks the configured lists about an address. It never returns an
// error: a list that cannot be reached must not stop mail from arriving.
func (m *EmailManager) checkDNSBL(ip net.IP, cfg EmailServerConfig) DNSBLResult {
	if !cfg.DNSBLEnabled || !dnsblCheckable(ip) {
		return DNSBLResult{}
	}

	zones := parseDNSBLZones(cfg.DNSBLZones)
	if len(zones) == 0 {
		return DNSBLResult{}
	}

	key := ip.String()
	if cached, ok := dnsblAnswers.get(key); ok {
		return cached
	}

	ctx, cancel := context.WithTimeout(context.Background(), dnsblTimeout)
	defer cancel()

	result := queryDNSBLZones(ctx, net.DefaultResolver, ip, zones)
	dnsblAnswers.put(key, result)
	return result
}

// queryDNSBLZones asks every list at once and reports the first hit. Split out
// from checkDNSBL so it can be tested against a resolver that answers locally.
func queryDNSBLZones(ctx context.Context, resolver *net.Resolver, ip net.IP, zones []string) DNSBLResult {
	type answer struct {
		zone   string
		listed bool
	}

	found := make(chan answer, len(zones))
	var wg sync.WaitGroup

	for _, zone := range zones {
		name := dnsblQueryName(ip, zone)
		if name == "" {
			continue
		}

		wg.Add(1)
		go func(zone, name string) {
			defer wg.Done()
			addrs, err := resolver.LookupHost(ctx, name)
			found <- answer{zone: zone, listed: err == nil && len(addrs) > 0}
		}(zone, name)
	}

	wg.Wait()
	close(found)

	for a := range found {
		if !a.listed {
			continue
		}
		return DNSBLResult{Listed: true, Zone: a.zone, Reason: dnsblReason(ctx, resolver, ip, a.zone)}
	}
	return DNSBLResult{}
}

// dnsblReason reads the TXT record a list publishes alongside a listing, which
// is usually a sentence and a link explaining why.
func dnsblReason(ctx context.Context, resolver *net.Resolver, ip net.IP, zone string) string {
	records, err := resolver.LookupTXT(ctx, dnsblQueryName(ip, zone))
	if err != nil || len(records) == 0 {
		return ""
	}
	return strings.TrimSpace(records[0])
}
