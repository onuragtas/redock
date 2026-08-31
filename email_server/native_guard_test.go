package email_server

import (
	"testing"
	"time"
)

func TestGuardBlocksAfterRepeatedAuthFailures(t *testing.T) {
	m := newTestManager(t)
	m.config.GuardEnabled = true
	m.config.MaxAuthFailures = 3
	m.config.BlockMinutes = 5

	const attacker = "203.0.113.77"

	for i := 0; i < 2; i++ {
		m.noteAuthFailure("smtp", attacker, "alice@example.com")
		if blocked, _ := m.guard().Blocked(attacker); blocked {
			t.Fatalf("blocked after only %d failures", i+1)
		}
	}

	m.noteAuthFailure("smtp", attacker, "alice@example.com")

	blocked, entry := m.guard().Blocked(attacker)
	if !blocked {
		t.Fatal("the address should be blocked once the limit is reached")
	}
	if entry.Failures < 3 {
		t.Errorf("the block should record the failure count: %+v", entry)
	}
	if !m.allowConnection("smtp", attacker) == false {
		t.Error("a blocked address must be refused at accept time")
	}
}

func TestGuardEnforcesTheConnectionRate(t *testing.T) {
	m := newTestManager(t)
	m.config.GuardEnabled = true
	m.config.MaxConnectionsPerMinute = 3
	m.config.BlockMinutes = 5

	const flooder = "203.0.113.88"

	for i := 0; i < 3; i++ {
		if !m.allowConnection("smtp", flooder) {
			t.Fatalf("connection %d should still be allowed", i+1)
		}
	}
	if m.allowConnection("smtp", flooder) {
		t.Fatal("the connection rate limit was not enforced")
	}
	if blocked, _ := m.guard().Blocked(flooder); !blocked {
		t.Error("exceeding the rate should block the address")
	}
}

func TestGuardNeverBlocksLocalAddresses(t *testing.T) {
	m := newTestManager(t)
	m.config.GuardEnabled = true
	m.config.MaxAuthFailures = 1
	m.config.MaxConnectionsPerMinute = 1

	for _, local := range []string{"127.0.0.1", "192.168.1.10", "10.1.2.3"} {
		for i := 0; i < 5; i++ {
			m.noteAuthFailure("imap", local, "alice@example.com")
			m.allowConnection("imap", local)
		}
		if blocked, _ := m.guard().Blocked(local); blocked {
			t.Errorf("%s is on the local network and must never be locked out", local)
		}
	}
}

func TestGuardCanBeDisabled(t *testing.T) {
	m := newTestManager(t)
	m.config.GuardEnabled = false
	m.config.MaxAuthFailures = 1

	const address = "203.0.113.99"
	for i := 0; i < 5; i++ {
		m.noteAuthFailure("smtp", address, "alice@example.com")
	}
	if blocked, _ := m.guard().Blocked(address); blocked {
		t.Error("nothing should be blocked while the guard is off")
	}
	if !m.allowConnection("smtp", address) {
		t.Error("connections must be allowed while the guard is off")
	}
}

func TestManualBlockAndUnblock(t *testing.T) {
	m := newTestManager(t)
	m.config.GuardEnabled = true

	if _, err := m.BlockClient("not-an-ip", "testing", 5); err == nil {
		t.Error("a malformed address must be refused")
	}

	entry, err := m.BlockClient("203.0.113.55", "spam source", 10)
	if err != nil {
		t.Fatalf("BlockClient: %v", err)
	}
	if !entry.Manual {
		t.Error("an operator block should be marked manual")
	}
	if entry.Until.Before(time.Now().Add(9 * time.Minute)) {
		t.Errorf("the block is shorter than requested: %+v", entry)
	}

	if list := m.BlockedClients(); len(list) != 1 {
		t.Fatalf("expected one block, got %d", len(list))
	}

	if err := m.UnblockClient("203.0.113.55"); err != nil {
		t.Fatalf("UnblockClient: %v", err)
	}
	if blocked, _ := m.guard().Blocked("203.0.113.55"); blocked {
		t.Error("the block should be gone")
	}
	if err := m.UnblockClient("203.0.113.55"); err == nil {
		t.Error("unblocking twice should report that nothing was blocked")
	}
}

func TestExpiredBlocksAreDropped(t *testing.T) {
	m := newTestManager(t)
	guard := m.guard()

	entry := guard.Block("203.0.113.44", "temporary", time.Millisecond, 1, false)
	if entry == nil {
		t.Fatal("the block was not created")
	}

	time.Sleep(5 * time.Millisecond)
	if blocked, _ := guard.Blocked("203.0.113.44"); blocked {
		t.Error("an expired block must not still refuse the address")
	}
	if len(guard.List()) != 0 {
		t.Error("expired blocks should not be listed")
	}
}
