package dns_server

import (
	"fmt"
	"log"
	"slices"

	"redock/platform/memguard"
	"redock/platform/memory"
)

const (
	// maxQueryLogRows caps the DNS query log table. Every row lives in RAM as a
	// Go struct, so an uncapped log table is the single biggest way this process
	// can eat memory: a few hundred thousand queries is already hundreds of MB.
	maxQueryLogRows = 50000
	// degradedQueryLogRows / emergencyQueryLogRows are what is kept when memory
	// gets tight — recent history stays queryable, the long tail goes.
	degradedQueryLogRows  = 10000
	emergencyQueryLogRows = 1000
	// avgQueryLogBytes is a rough per-row cost used to report reclaimed bytes.
	avgQueryLogBytes = 320
)

// ApplyMemoryLimits caps the log table and registers the DNS relievers with the
// memory guard. Called from Init once the table is registered.
func ApplyMemoryLimits(db *memory.Database) {
	if db == nil {
		return
	}

	if dropped := memory.SetTableLimit(db, dnsQueryLogsTable, maxQueryLogRows); dropped > 0 {
		log.Printf("DNS query log capped at %d rows, %d old rows dropped", maxQueryLogRows, dropped)
	}

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "dns-query-logs",
		Description: "Drops old DNS query log rows kept in memory",
		MinLevel:    memguard.LevelWarning,
		Priority:    20,
		Release: func(level memguard.Level) (int64, string) {
			keep := degradedQueryLogRows
			if level >= memguard.LevelEmergency {
				keep = emergencyQueryLogRows
			}
			dropped := memory.TrimTable(db, dnsQueryLogsTable, keep)
			if dropped == 0 {
				return 0, ""
			}
			return int64(dropped) * avgQueryLogBytes, fmt.Sprintf("%d query log rows dropped, %d kept", dropped, keep)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "dns-cache",
		Description: "Clears the DNS response cache",
		MinLevel:    memguard.LevelCritical,
		Priority:    30,
		Release: func(memguard.Level) (int64, string) {
			s := GetServer()
			if s == nil || s.cache == nil {
				return 0, ""
			}
			size := s.cache.GetSize()
			if size == 0 {
				return 0, ""
			}
			s.cache.Clear()
			// A cached response averages a few hundred bytes once parsed.
			return int64(size) * 512, fmt.Sprintf("%d cached DNS responses evicted", size)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "dns-stats",
		Description: "Trims per-domain/per-client counters down to the top entries",
		MinLevel:    memguard.LevelCritical,
		Priority:    40,
		Release: func(memguard.Level) (int64, string) {
			s := GetServer()
			if s == nil || s.stats == nil {
				return 0, ""
			}
			dropped := s.stats.TrimCounters(500)
			if dropped == 0 {
				return 0, ""
			}
			return int64(dropped) * 64, fmt.Sprintf("%d stat counters dropped", dropped)
		},
	})
}

// TrimCounters keeps only the top `keep` entries of each counter map and
// reports how many entries were dropped. Unbounded domain/client maps are a
// slow leak on a busy resolver.
func (s *StatsCollector) TrimCounters(keep int) int {
	s.topDomainsMutex.Lock()
	defer s.topDomainsMutex.Unlock()

	dropped := 0
	dropped += trimCounterMap(s.topDomains, keep)
	dropped += trimCounterMap(s.topBlocked, keep)
	dropped += trimCounterMap(s.topClients, keep)
	return dropped
}

// trimCounterMap drops the lowest-count entries until at most keep remain.
func trimCounterMap(m map[string]int64, keep int) int {
	if len(m) <= keep {
		return 0
	}

	// Find the count threshold that leaves roughly `keep` entries, then delete
	// everything below it — cheaper than sorting the keys themselves.
	counts := make([]int64, 0, len(m))
	for _, c := range m {
		counts = append(counts, c)
	}
	slices.Sort(counts) // ascending
	threshold := counts[len(counts)-keep]

	dropped := 0
	for k, c := range m {
		if c < threshold {
			delete(m, k)
			dropped++
		}
	}
	return dropped
}
