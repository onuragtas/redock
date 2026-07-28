//go:build darwin

package traffic_inspector

import (
	"strings"
	"testing"
)

// vpnServerAnchorTemplate mirrors the exact anchor file content vpn_server's
// setupNAT (manager.go) writes for a darwin server — kept in sync manually
// since traffic_inspector doesn't import vpn_server's private helper.
const vpnServerAnchorTemplate = `# Redock VPN rules for utun107
nat on en0 inet from 10.0.0.0/24 to any -> (en0)

pass in quick on utun107 inet from 10.0.0.0/24 to any keep state
pass out quick on utun107 inet from 10.0.0.0/24 to any keep state
`

func TestInsertBeforeFilterRules_KeepsTranslationBeforeFiltering(t *testing.T) {
	block := inspectBlockBegin + "\n" +
		"rdr pass on utun107 proto tcp from 10.0.0.2 to any -> 127.0.0.1 port 20001\n" +
		inspectBlockEnd + "\n"

	result := insertBeforeFilterRules(vpnServerAnchorTemplate, block)

	natIdx := strings.Index(result, "nat on")
	rdrIdx := strings.Index(result, "rdr pass")
	passIdx := strings.Index(result, "pass in quick")

	if natIdx == -1 || rdrIdx == -1 || passIdx == -1 {
		t.Fatalf("expected nat, rdr, and pass lines all present in result:\n%s", result)
	}
	if !(natIdx < rdrIdx && rdrIdx < passIdx) {
		t.Fatalf("pf ordering violated (want nat < rdr < pass), got offsets nat=%d rdr=%d pass=%d:\n%s", natIdx, rdrIdx, passIdx, result)
	}
}

func TestInsertBeforeFilterRules_EmptyBlockJustTrims(t *testing.T) {
	result := insertBeforeFilterRules(vpnServerAnchorTemplate, "")
	if strings.Contains(result, inspectBlockBegin) {
		t.Fatalf("expected no managed block markers when block is empty, got:\n%s", result)
	}
	if !strings.Contains(result, "pass out quick") {
		t.Fatalf("expected original filter rules preserved, got:\n%s", result)
	}
}

func TestStripManagedBlock_RemovesPreviousBlockCleanly(t *testing.T) {
	withBlock := vpnServerAnchorTemplate[:strings.Index(vpnServerAnchorTemplate, "pass in quick")] +
		inspectBlockBegin + "\n" +
		"rdr pass on utun107 proto tcp from 10.0.0.2 to any -> 127.0.0.1 port 20001\n" +
		inspectBlockEnd + "\n" +
		vpnServerAnchorTemplate[strings.Index(vpnServerAnchorTemplate, "pass in quick"):]

	stripped := stripManagedBlock(withBlock)
	if strings.Contains(stripped, "rdr pass") || strings.Contains(stripped, inspectBlockBegin) {
		t.Fatalf("expected managed block fully removed, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "pass in quick") || !strings.Contains(stripped, "nat on") {
		t.Fatalf("expected original rules preserved after stripping, got:\n%s", stripped)
	}
}
