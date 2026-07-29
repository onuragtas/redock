//go:build linux

package traffic_inspector

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
)

// ensureLocalnetRouting enables net.ipv4.conf.<iface>.route_localnet on the
// VPN tun interface (and `all`). Without this, the kernel treats forwarded
// packets whose destination the REDIRECT rule rewrites to 127.0.0.1 as
// martians and silently drops them *after* the NAT rewrite — so the iptables
// counters increment but nothing ever reaches the loopback MITM listener.
// This is the Linux-specific reason transparent REDIRECT-to-loopback works
// on macOS (pf) but captures nothing here. Idempotent; safe to call per rule.
func ensureLocalnetRouting(tunIface string) {
	exec.Command("sysctl", "-w", "net.ipv4.conf.all.route_localnet=1").Run()
	exec.Command("sysctl", "-w", fmt.Sprintf("net.ipv4.conf.%s.route_localnet=1", tunIface)).Run()
}

// AddUserRedirect adds an iptables NAT PREROUTING rule that redirects a
// single inspected user's TCP traffic (matched by source IP, on the VPN's
// tun interface) to our local TCP MITM listener. The real destination is
// recovered later via SO_ORIGINAL_DST (see natlookup_linux.go).
func AddUserRedirect(tunIface, userIP string, proxyPort int) error {
	ensureLocalnetRouting(tunIface)

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
	// DNAT (not REDIRECT) to an explicit 127.0.0.1 destination. For forwarded
	// traffic (PREROUTING) `-j REDIRECT` rewrites the destination to the
	// *incoming interface's* primary IP (the tun's IP, e.g. 10.0.0.1) — never
	// 127.0.0.1 — so a loopback-bound listener never sees the packet. DNAT
	// lets us target 127.0.0.1 directly (matching the macOS pf `rdr -> 127.0.0.1`
	// semantics), which reaches the listener once route_localnet is enabled.
	return []string{
		"PREROUTING",
		"-i", tunIface,
		"-s", userIP,
		"-p", "tcp",
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("127.0.0.1:%d", proxyPort),
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
		// Strict reverse-path filtering drops TPROXY-diverted packets (the
		// client's source IP does not route back out the tun interface).
		// Relax to loose mode (2); the kernel uses max(all, iface), so `all`
		// must be lowered too — the per-tun value is set in AddUserRedirectQUIC.
		exec.Command("sysctl", "-w", "net.ipv4.conf.all.rp_filter=2").Run()
	})
}

// AddUserRedirectQUIC adds an iptables mangle PREROUTING TPROXY rule that
// transparently redirects a single inspected user's UDP traffic (matched by
// source IP, on the VPN's tun interface) to our local QUIC MITM listener,
// preserving the original destination (recovered via IP_ORIGDSTADDR, see
// natlookup_linux.go's UDP counterpart in udp_transparent_linux.go).
func AddUserRedirectQUIC(tunIface, userIP string, proxyPort int) error {
	ensureTPROXYRouting()
	// Loose reverse-path filtering on the tun iface so TPROXY-diverted UDP
	// (QUIC) from the client is not dropped before local delivery.
	exec.Command("sysctl", "-w", fmt.Sprintf("net.ipv4.conf.%s.rp_filter=2", tunIface)).Run()

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
