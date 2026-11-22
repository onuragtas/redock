package dns_server

import (
	"fmt"
	"net"
	"strings"
)

// clientACL is the set of networks allowed to query the server (DNSConfig.AllowedClients).
type clientACL struct {
	nets []*net.IPNet
}

// parseClientACL turns "10.0.0.0/8" / "192.168.1.5" entries into networks; entries that are neither are
// returned as errors and skipped (an unparsable entry never widens access).
func parseClientACL(entries []string) (clientACL, []error) {
	var acl clientACL
	var errs []error
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			ip := net.ParseIP(e)
			if ip == nil {
				errs = append(errs, fmt.Errorf("invalid client address %q", e))
				continue
			}
			bits := 128
			if ip.To4() != nil {
				ip, bits = ip.To4(), 32
			}
			acl.nets = append(acl.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid client network %q", e))
			continue
		}
		acl.nets = append(acl.nets, n)
	}
	return acl, errs
}

// allows reports whether ip is inside one of the networks; an unknown address is refused.
func (a clientACL) allows(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range a.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateAllowedClients checks a list before it is saved.
func ValidateAllowedClients(entries []string) error {
	_, errs := parseClientACL(entries)
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
