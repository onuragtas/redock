package memguard

import "testing"

// TestDetectSystemMemorySmoke is a sanity check that the platform probes work
// on the host running the tests.
func TestDetectSystemMemorySmoke(t *testing.T) {
	total, source := detectSystemMemory()
	if total <= 0 {
		t.Fatalf("could not detect system memory (source %q)", source)
	}
	t.Logf("detected %s of memory via %s", humanBytes(total), source)

	rss := processRSS()
	if rss <= 0 {
		t.Fatalf("could not read this process' RSS")
	}
	t.Logf("this process RSS: %s", humanBytes(rss))
}
