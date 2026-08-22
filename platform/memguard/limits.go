package memguard

import (
	"os"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// detectSystemMemory returns the amount of memory the process may realistically
// use and where that number came from ("cgroup", "system", "unknown").
func detectSystemMemory() (int64, string) {
	if limit, ok := cgroupMemoryLimit(); ok && limit > 0 {
		return limit, "cgroup"
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm.Total > 0 {
		return int64(vm.Total), "system"
	}
	return 0, "unknown"
}

var selfProc *process.Process

// processRSS returns the resident set size of this process — the number the
// kernel OOM killer actually scores — or 0 when it cannot be read.
func processRSS() int64 {
	if selfProc == nil {
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			return 0
		}
		selfProc = p
	}
	info, err := selfProc.MemoryInfo()
	if err != nil || info == nil {
		return 0
	}
	return int64(info.RSS)
}
