package email_server

import (
	"log"
	"strings"
	"time"
)

// A message that has been sent leaves no trace in the sender's own mailbox
// unless somebody puts it there. SMTP hands a message to the next hop and is
// finished; the copy in "Sent" is a courtesy, and on a desktop client it is
// the client that performs it, with a separate IMAP APPEND after submission.
//
// Webmail has no such second half, and neither does a script that just opens
// an SMTP connection and sends. For those, mail left the building and the Sent
// folder stayed empty — the message was never stored, not stored and lost.
// The server therefore files the sender's copy itself.

const (
	sentName = "Sent"

	// sentDedupeWindow bounds the search for an existing copy. A client that
	// saves its own copy does so seconds after submission, so a short window
	// catches the duplicate while keeping the scan cheap on a large folder.
	sentDedupeWindow = 15 * time.Minute
)

// saveToSent files the sender's copy of an outgoing message, already read.
//
// The message has been accepted for delivery by the time this runs, so a
// failure here must not fail the send: the copy is worth a log line, never a
// bounce for mail that is already on its way.
func (m *EmailManager) saveToSent(account *Account, raw []byte) {
	if account == nil || account.Mailbox == nil || len(raw) == 0 {
		return
	}

	// A client may have saved its own copy first; do not file a second one.
	if m.sentHasMessageID(account, messageIDOf(raw)) {
		return
	}

	if err := m.deliverLocal(account, sentName, raw, []string{imapFlagSeen}); err != nil {
		log.Printf("mail: could not file the Sent copy for %s: %v", account.Address(), err)
	}
}

// sentHasMessageID reports whether the Sent folder already holds this message.
//
// Message-ID is the only identity that survives both routes into the folder:
// the server writes the message it submitted, while a client appends the copy
// it composed, and the two differ in their trace headers but never in this one.
func (m *EmailManager) sentHasMessageID(account *Account, messageID string) bool {
	if messageID == "" || account == nil {
		return false
	}

	messages, err := m.store().List(account.Base, sentName)
	if err != nil {
		return false
	}

	// Newest first: UIDs are handed out in arrival order, and the copy we are
	// looking for would have arrived moments ago.
	cutoff := time.Now().Add(-sentDedupeWindow)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Date.Before(cutoff) {
			break
		}
		raw, err := m.store().Read(account.Base, sentName, messages[i])
		if err != nil {
			continue
		}
		if messageIDOf(raw) == messageID {
			return true
		}
	}
	return false
}

// messageIDOf reads the Message-ID header, without the angle brackets that
// clients disagree about including.
func messageIDOf(raw []byte) string {
	value := strings.TrimSpace(headerValue(raw, "Message-ID"))
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return strings.TrimSpace(value)
}
