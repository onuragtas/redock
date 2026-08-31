package email_server

import (
	"fmt"

	"redock/platform/memguard"
)

// The mail engine keeps three things in RAM that grow with traffic rather than
// with configuration: the protocol traces behind the Connections log, the
// per-address counters the abuse guard rates clients on, and the record of who
// has already had an auto-reply. The email_logs table is capped elsewhere with
// the other log tables; these are plain Go structures and need their own way
// back down when memory gets tight.
//
// What each reliever gives up is chosen deliberately. Traces are diagnostics
// and can go first. Guard counters rebuild themselves from the next few
// connections. The blocks those counters led to are NOT dropped: forgetting a
// block would let an attacker straight back in, which is a security decision,
// not a memory one.

const (
	// tracesKeptUnderPressure is how many recent connections stay readable when
	// memory is tight. Enough to still see what just happened.
	tracesKeptUnderPressure = 40
	// avgTraceBytes is a rough per-trace cost, used to report reclaimed bytes.
	// A trace is up to 400 lines but a typical SMTP conversation is far shorter.
	avgTraceBytes = 4 * 1024
	// avgGuardEntryBytes is a rough cost per tracked address.
	avgGuardEntryBytes = 128
	// avgAutoReplyEntryBytes is a rough cost per remembered correspondent.
	avgAutoReplyEntryBytes = 96
)

// RegisterMemoryRelievers hooks the mail engine into the memory guard. Called
// from Init once the manager exists.
func RegisterMemoryRelievers(m *EmailManager) {
	if m == nil {
		return
	}

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "mail-connection-traces",
		Description: "Drops stored SMTP/IMAP/POP3 connection traces",
		MinLevel:    memguard.LevelWarning,
		Priority:    25,
		Release: func(level memguard.Level) (int64, string) {
			keep := tracesKeptUnderPressure
			if level >= memguard.LevelEmergency {
				keep = 0
			}
			dropped, emptied := m.traces().trim(keep)
			if dropped+emptied == 0 {
				return 0, ""
			}
			// An emptied trace gives up its lines but keeps its header, so it
			// releases a little less than one that goes entirely.
			reclaimed := int64(dropped)*avgTraceBytes + int64(emptied)*(avgTraceBytes-256)
			return reclaimed, fmt.Sprintf(
				"%d connection traces dropped, %d open ones emptied, %d kept in full",
				dropped, emptied, keep)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "mail-guard-counters",
		Description: "Clears per-address connection and auth-failure counters (blocks are kept)",
		MinLevel:    memguard.LevelCritical,
		Priority:    35,
		Release: func(memguard.Level) (int64, string) {
			guard := m.Native().clientGuard()
			if guard == nil {
				return 0, ""
			}
			dropped := guard.clearCounters()
			if dropped == 0 {
				return 0, ""
			}
			return int64(dropped) * avgGuardEntryBytes,
				fmt.Sprintf("%d tracked addresses forgotten, active blocks kept", dropped)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "mail-auto-reply-memory",
		Description: "Forgets who has already received an auto-reply",
		MinLevel:    memguard.LevelCritical,
		Priority:    45,
		Release: func(memguard.Level) (int64, string) {
			// The cost of forgetting is that one correspondent may get a second
			// vacation notice; cheap next to running out of memory.
			dropped := autoReplies.clear()
			if dropped == 0 {
				return 0, ""
			}
			return int64(dropped) * avgAutoReplyEntryBytes,
				fmt.Sprintf("%d auto-reply records forgotten", dropped)
		},
	})
}
