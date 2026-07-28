//go:build linux

package traffic_inspector

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
)

// AddUserRedirect adds an iptables NAT PREROUTING rule that redirects a
// single inspected user's TCP traffic (matched by source IP, on the VPN's
// tun interface) to our local TCP MITM listener. The real destination is
// recovered later via SO_ORIGINAL_DST (see natlookup_linux.go).
func AddUserRedirect(tunIface, userIP string, proxyPort int) error {
	args := redirectRuleArgs(tunIface, userIP, proxyPort)

	checkArgs := append([]string{"-t", "nat", "-C"}, args...)
	if exec.Command("iptables", checkArgs...).Run() == nil {
		return nil // already present
	}

	addArgs := append([]string{"-t", "nat", "-A"}, args...)
	out, err := exec.Command("iptables", addArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables add redirect failed: %w, output: %s", err, string(out))
	}
	return nil
}

// RemoveUserRedirect removes the redirect rule added by AddUserRedirect.
func RemoveUserRedirect(tunIface, userIP string, proxyPort int) error {
	args := redirectRuleArgs(tunIface, userIP, proxyPort)

	delArgs := append([]string{"-t", "nat", "-D"}, args...)
	out, err := exec.Command("iptables", delArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables remove redirect failed: %w, output: %s", err, string(out))
	}
	return nil
}

func redirectRuleArgs(tunIface, userIP string, proxyPort int) []string {
	return []string{
		"PREROUTING",
		"-i", tunIface,
		"-s", userIP,
		"-p", "tcp",
		"-j", "REDIRECT",
		"--to-port", strconv.Itoa(proxyPort),
	}
}

var tproxyRoutingOnce sync.Once

// ensureTPROXYRouting performs the one-time policy-routing setup TPROXY
// needs to deliver marked packets to a local socket: a rule sending
// fwmark-1 traffic to table 100, and a local default route in that table.
// Idempotent — safe to call on every QUIC interceptor start.
func ensureTPROXYRouting() {
	tproxyRoutingOnce.Do(func() {
		exec.Command("ip", "rule", "add", "fwmark", "1", "lookup", "100").Run()
		exec.Command("ip", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", "100").Run()
	})
}

// AddUserRedirectQUIC adds an iptables mangle PREROUTING TPROXY rule that
// transparently redirects a single inspected user's UDP traffic (matched by
// source IP, on the VPN's tun interface) to our local QUIC MITM listener,
// preserving the original destination (recovered via IP_ORIGDSTADDR, see
// natlookup_linux.go's UDP counterpart in udp_transparent_linux.go).
func AddUserRedirectQUIC(tunIface, userIP string, proxyPort int) error {
	ensureTPROXYRouting()

	args := tproxyRuleArgs(tunIface, userIP, proxyPort)

	checkArgs := append([]string{"-t", "mangle", "-C"}, args...)
	if exec.Command("iptables", checkArgs...).Run() == nil {
		return nil // already present
	}

	addArgs := append([]string{"-t", "mangle", "-A"}, args...)
	out, err := exec.Command("iptables", addArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables TPROXY add failed: %w, output: %s", err, string(out))
	}
	return nil
}

// RemoveUserRedirectQUIC removes the TPROXY rule added by AddUserRedirectQUIC.
func RemoveUserRedirectQUIC(tunIface, userIP string, proxyPort int) error {
	args := tproxyRuleArgs(tunIface, userIP, proxyPort)

	delArgs := append([]string{"-t", "mangle", "-D"}, args...)
	out, err := exec.Command("iptables", delArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables TPROXY remove failed: %w, output: %s", err, string(out))
	}
	return nil
}

// quicUDPPort is the UDP port QUIC/HTTP3 traffic is redirected from.
// Deliberately scoped to 443 (unlike the TCP redirect, which is
// intentionally port-agnostic for arbitrary custom TCP+TLS protocols) —
// quic-go's listener silently discards any non-QUIC datagram it receives,
// so blanket-redirecting all UDP would blackhole DNS (UDP/53) and any other
// non-QUIC UDP traffic from the inspected user. A custom QUIC-based
// protocol running on a non-443 port would not be caught by this rule.
const quicUDPPort = 443

func tproxyRuleArgs(tunIface, userIP string, proxyPort int) []string {
	return []string{
		"PREROUTING",
		"-i", tunIface,
		"-s", userIP,
		"-p", "udp",
		"--dport", strconv.Itoa(quicUDPPort),
		"-j", "TPROXY",
		"--on-port", strconv.Itoa(proxyPort),
		"--on-ip", "127.0.0.1",
		"--tproxy-mark", "1",
	}
}
