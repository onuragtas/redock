//go:build linux

package memguard

import (
	"os"
	"strconv"
	"strings"
)

// cgroupMemoryLimit reads the container memory limit (cgroup v2 first, then
// v1). Returns ok=false when the process is not memory-limited by a cgroup.
func cgroupMemoryLimit() (int64, bool) {
	// cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		raw := strings.TrimSpace(string(data))
		if raw != "max" {
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
				return v, true
			}
		}
	}
	// cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && v > 0 {
			// Unlimited is reported as a huge sentinel value (~2^63 page-aligned).
			if v < (1 << 60) {
				return v, true
			}
		}
	}
	return 0, false
}

// protectFromOOMKiller lowers this process' OOM score so the kernel evicts
// something else first when the host runs out of memory.
func protectFromOOMKiller() (string, bool) {
	const score = "-800"
	if err := os.WriteFile("/proc/self/oom_score_adj", []byte(score), 0644); err != nil {
		return "oom_score_adj could not be set: " + err.Error(), false
	}
	return "oom_score_adj=" + score, true
}
