package network

import (
	"log"
	"runtime"

	"redock/platform/memory"
)

const TableIPAliases = "network_ip_aliases"

// PersistedIPAlias, IP alias'ın kalıcı kaydı (memory DB). Redock açılışında kernel'e tekrar uygulanır.
type PersistedIPAlias struct {
	memory.BaseEntity
	Interface   string `json:"interface"`
	CIDROrRange string `json:"cidr_or_range"`
}

// ApplyPersistedAliases, memory DB'deki tüm kayıtlı alias'ları Linux kernel'e uygular (Redock açılışında çağrılır).
func ApplyPersistedAliases(db *memory.Database) {
	if db == nil {
		return
	}
	if runtime.GOOS != "linux" {
		return
	}
	all := memory.FindAll[*PersistedIPAlias](db, TableIPAliases)
	for _, a := range all {
		if a.Interface == "" || a.CIDROrRange == "" {
			continue
		}
		ipNets, err := ParseIPRange(a.CIDROrRange)
		if err != nil {
			log.Printf("network: persisted alias apply parse %q: %v", a.CIDROrRange, err)
			continue
		}
		added, err := AddAliases(a.Interface, ipNets)
		if err != nil {
			log.Printf("network: persisted alias apply %s %q: %v", a.Interface, a.CIDROrRange, err)
			continue
		}
		log.Printf("network: persisted aliases applied %s %q (%d addresses)", a.Interface, a.CIDROrRange, added)
	}
}
