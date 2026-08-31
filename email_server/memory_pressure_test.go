package email_server

import (
	"testing"
	"time"
)

func TestTraceTrimKeepsTheNewest(t *testing.T) {
	store := newTraceStore()
	base := time.Now()
	for i := 1; i <= 10; i++ {
		store.begin(&ConnectionTrace{ID: uint64(i), Service: "smtp", StartedAt: base.Add(time.Duration(i) * time.Second)})
		store.end(uint64(i), "") // finished connections, the usual case
	}

	dropped, emptied := store.trim(3)
	if dropped != 7 || emptied != 0 {
		t.Fatalf("trim dropped %d and emptied %d, want 7 and 0", dropped, emptied)
	}

	list := store.List(0)
	if len(list) != 3 {
		t.Fatalf("kept %d traces, want 3", len(list))
	}
	// Still newest first, and the newest are the ones that survived.
	for i, wantID := range []uint64{10, 9, 8} {
		if list[i].ID != wantID {
			t.Errorf("traces[%d].ID = %d, want %d", i, list[i].ID, wantID)
		}
	}

	// Trimming again changes nothing, and the ring still accepts new entries.
	if dropped, emptied := store.trim(3); dropped != 0 || emptied != 0 {
		t.Errorf("second trim dropped %d and emptied %d, want 0 and 0", dropped, emptied)
	}
	store.begin(&ConnectionTrace{ID: 11, Service: "smtp", StartedAt: base.Add(time.Minute)})
	if list := store.List(0); len(list) != 4 || list[0].ID != 11 {
		t.Errorf("after trimming, a new trace did not land on top: %v", list)
	}
}

func TestTraceTrimToZeroClearsEverything(t *testing.T) {
	store := newTraceStore()
	for i := 1; i <= 5; i++ {
		store.begin(&ConnectionTrace{ID: uint64(i)})
		store.end(uint64(i), "")
	}
	if dropped, _ := store.trim(0); dropped != 5 {
		t.Fatalf("trim dropped %d, want 5", dropped)
	}
	if list := store.List(0); len(list) != 0 {
		t.Errorf("still holding %d traces", len(list))
	}
}

// A connection still being written to must survive a trim, or the log loses
// the conversation that is happening right now.
func TestTraceTrimKeepsLiveConnectionsButEmptiesThem(t *testing.T) {
	store := newTraceStore()
	for i := 1; i <= 5; i++ {
		store.begin(&ConnectionTrace{ID: uint64(i)})
		store.append(uint64(i), "in", "EHLO client.example.com")
		if i > 1 {
			store.end(uint64(i), "") // only connection 1 stays open
		}
	}

	dropped, emptied := store.trim(1)
	if emptied != 1 {
		t.Errorf("emptied %d live traces, want 1", emptied)
	}
	if dropped != 3 {
		t.Errorf("dropped %d finished traces, want 3", dropped)
	}

	var live *ConnectionTrace
	for i, trace := range store.List(0) {
		if trace.ID == 1 {
			live = &store.List(0)[i]
		}
	}
	if live == nil {
		t.Fatal("the open connection's trace was dropped")
	}
	if len(live.Lines) != 0 || !live.Truncated {
		t.Errorf("the open trace kept its lines instead of giving them up: %+v", live)
	}
}

// Clearing the counters must not release the clients already blocked.
func TestGuardClearCountersKeepsBlocks(t *testing.T) {
	guard := newConnectionGuard()
	guard.failures["203.0.113.5"] = []time.Time{time.Now()}
	guard.connections["203.0.113.6"] = []time.Time{time.Now()}
	guard.blocked["203.0.113.7"] = &BlockedClient{IP: "203.0.113.7", Until: time.Now().Add(time.Hour)}

	if dropped := guard.clearCounters(); dropped != 2 {
		t.Fatalf("cleared %d addresses, want 2", dropped)
	}
	if len(guard.failures) != 0 || len(guard.connections) != 0 {
		t.Error("counters survived the clear")
	}
	if _, ok := guard.blocked["203.0.113.7"]; !ok {
		t.Error("clearing counters released an active block")
	}
}

// Expiring by age is not a bound when every entry is fresh, which is exactly
// what a flood of new senders produces.
func TestAutoReplyMemoryStaysBoundedUnderAFlood(t *testing.T) {
	memory := &autoReplyMemory{sent: make(map[string]time.Time)}

	for i := 0; i < autoReplyMaxEntries*2; i++ {
		memory.shouldReply("alice@example.com", "sender"+time.Duration(i).String()+"@example.com")
	}

	if len(memory.sent) > autoReplyMaxEntries {
		t.Errorf("auto-reply memory grew to %d entries, ceiling is %d", len(memory.sent), autoReplyMaxEntries)
	}

	if dropped := memory.clear(); dropped == 0 {
		t.Error("clear reported nothing to forget")
	}
	if len(memory.sent) != 0 {
		t.Errorf("clear left %d entries", len(memory.sent))
	}
}
