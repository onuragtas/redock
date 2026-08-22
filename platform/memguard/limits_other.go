//go:build !linux

package memguard

// cgroupMemoryLimit has no meaning outside Linux.
func cgroupMemoryLimit() (int64, bool) { return 0, false }

// protectFromOOMKiller is a no-op outside Linux; macOS/Windows expose no
// equivalent knob for a process to deprioritise itself.
func protectFromOOMKiller() (string, bool) {
	return "oom protection is only available on Linux", false
}
