//go:build darwin

package traffic_inspector

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

const inspectBlockBegin = "# --- redock traffic inspector rdr rules (managed, do not edit) ---"
const inspectBlockEnd = "# --- end redock traffic inspector rdr rules ---"

var pfMu sync.Mutex

// redirects/redirectsUDP track, per tun interface, the userIP->proxyPort
// rules currently applied for each protocol, so the managed block can be
// regenerated wholesale on every add/remove without parsing rule text back
// out of the anchor file.
var redirects = make(map[string]map[string]int)
var redirectsUDP = make(map[string]map[string]int)
var redirectsMu sync.Mutex

// anchorFilePath reuses vpn_server's own pf anchor file for this tun
// interface (see vpn_server/manager.go setupNAT) rather than creating a
// second anchor — that file is already wired into /etc/pf.conf via matching
// nat-anchor/rdr-anchor/anchor lines, so appending rdr rules here needs no
// changes to pf.conf itself, only a reload.
func anchorFilePath(tunIface string) string {
	return fmt.Sprintf("/etc/pf.anchors/redock.vpn.%s", tunIface)
}

// AddUserRedirect appends (or updates) a TCP `rdr pass` rule for the given
// user IP into the VPN server's existing pf anchor file, then reloads pf.
func AddUserRedirect(tunIface, userIP string, proxyPort int) error {
	redirectsMu.Lock()
	if redirects[tunIface] == nil {
		redirects[tunIface] = make(map[string]int)
	}
	redirects[tunIface][userIP] = proxyPort
	tcpSnap, udpSnap := snapshotBoth(tunIface)
	redirectsMu.Unlock()

	return applyAnchorBlock(tunIface, tcpSnap, udpSnap)
}

// RemoveUserRedirect removes the TCP rule for the given user IP.
func RemoveUserRedirect(tunIface, userIP string, proxyPort int) error {
	redirectsMu.Lock()
	if m, ok := redirects[tunIface]; ok {
		delete(m, userIP)
	}
	tcpSnap, udpSnap := snapshotBoth(tunIface)
	redirectsMu.Unlock()

	return applyAnchorBlock(tunIface, tcpSnap, udpSnap)
}

// AddUserRedirectQUIC appends (or updates) a UDP `rdr pass` rule (used for
// QUIC interception) for the given user IP.
func AddUserRedirectQUIC(tunIface, userIP string, proxyPort int) error {
	redirectsMu.Lock()
	if redirectsUDP[tunIface] == nil {
		redirectsUDP[tunIface] = make(map[string]int)
	}
	redirectsUDP[tunIface][userIP] = proxyPort
	tcpSnap, udpSnap := snapshotBoth(tunIface)
	redirectsMu.Unlock()

	return applyAnchorBlock(tunIface, tcpSnap, udpSnap)
}

// RemoveUserRedirectQUIC removes the UDP rule for the given user IP.
func RemoveUserRedirectQUIC(tunIface, userIP string, proxyPort int) error {
	redirectsMu.Lock()
	if m, ok := redirectsUDP[tunIface]; ok {
		delete(m, userIP)
	}
	tcpSnap, udpSnap := snapshotBoth(tunIface)
	redirectsMu.Unlock()

	return applyAnchorBlock(tunIface, tcpSnap, udpSnap)
}

// snapshotBoth must be called with redirectsMu held.
func snapshotBoth(tunIface string) (map[string]int, map[string]int) {
	tcpOut := make(map[string]int, len(redirects[tunIface]))
	maps.Copy(tcpOut, redirects[tunIface])

	udpOut := make(map[string]int, len(redirectsUDP[tunIface]))
	maps.Copy(udpOut, redirectsUDP[tunIface])

	return tcpOut, udpOut
}

// quicUDPPort is the UDP port QUIC/HTTP3 rdr rules are scoped to (see
// redirect_linux.go's tproxyRuleArgs for the matching Linux-side rationale:
// blanket-redirecting all UDP would blackhole DNS and any other non-QUIC
// UDP traffic, since our QUIC listener silently discards non-QUIC packets).
const quicUDPPort = 443

func applyAnchorBlock(tunIface string, tcpRules, udpRules map[string]int) error {
	pfMu.Lock()
	defer pfMu.Unlock()

	path := anchorFilePath(tunIface)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read anchor file %s (is the VPN server running?): %w", path, err)
	}

	base := stripManagedBlock(string(data))

	var block strings.Builder
	if len(tcpRules) > 0 || len(udpRules) > 0 {
		block.WriteString(inspectBlockBegin + "\n")
		for ip, port := range tcpRules {
			fmt.Fprintf(&block, "rdr pass on %s proto tcp from %s to any -> 127.0.0.1 port %d\n", tunIface, ip, port)
		}
		for ip, port := range udpRules {
			fmt.Fprintf(&block, "rdr pass on %s proto udp from %s to any port %d -> 127.0.0.1 port %d\n", tunIface, ip, quicUDPPort, port)
		}
		block.WriteString(inspectBlockEnd + "\n")
	}

	newContent := insertBeforeFilterRules(base, block.String())
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("write anchor file: %w", err)
	}

	out, err := exec.Command("pfctl", "-f", "/etc/pf.conf").CombinedOutput()
	if err != nil {
		return fmt.Errorf("pfctl reload failed: %w, output: %s", err, string(out))
	}
	return nil
}

// insertBeforeFilterRules inserts the managed rdr block just before the
// first filter rule (pass/block) in the anchor file. pf requires strict
// ordering within a ruleset — options, normalization, queueing,
// translation (nat/rdr), filtering (pass/block) — and vpn_server's anchor
// (see vpn_server/manager.go setupNAT) already contains both a `nat` line
// and `pass in/out quick` lines; appending rdr rules after those `pass`
// lines is a translation-after-filtering ordering violation that pfctl
// rejects outright ("Rules must be in order"). Falls back to appending at
// the end if no filter rule line is found (still valid pf, just without an
// existing filter section to stay ahead of).
func insertBeforeFilterRules(base, block string) string {
	trimmedBase := strings.TrimRight(base, "\n")
	if block == "" {
		return trimmedBase + "\n"
	}

	lines := strings.Split(trimmedBase, "\n")
	insertAt := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pass") || strings.HasPrefix(trimmed, "block") {
			insertAt = i
			break
		}
	}

	var out strings.Builder
	if insertAt > 0 {
		out.WriteString(strings.Join(lines[:insertAt], "\n"))
		out.WriteString("\n")
	}
	out.WriteString(block)
	out.WriteString("\n")
	if insertAt < len(lines) {
		out.WriteString(strings.Join(lines[insertAt:], "\n"))
		out.WriteString("\n")
	}
	return out.String()
}

var managedBlockRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(inspectBlockBegin) + `.*?` + regexp.QuoteMeta(inspectBlockEnd) + `\n?`)

func stripManagedBlock(content string) string {
	return managedBlockRe.ReplaceAllString(content, "")
}
