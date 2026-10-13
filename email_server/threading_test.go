package email_server

import (
	"testing"
	"time"
)

func threadOf(threads []*EmailThread, uid uint32) *EmailThread {
	for _, th := range threads {
		for _, m := range th.Messages {
			if m.UID == uid {
				return th
			}
		}
	}
	return nil
}

func at(min int) time.Time { return time.Date(2026, 1, 1, 0, min, 0, 0, time.UTC) }

// Sent folder: same subject to different recipients must not merge.
func TestThreadsSameSubjectDifferentRecipients(t *testing.T) {
	emails := []*Email{
		{UID: 1, MessageID: "<a@x>", From: "Me <me@x.com>", To: "bob@y.com", Subject: "Fatura", Date: at(1)},
		{UID: 2, MessageID: "<b@x>", From: "Me <me@x.com>", To: "Alice <alice@z.com>", Subject: "Fatura", Date: at(2)},
		{UID: 3, MessageID: "<c@x>", From: "me@x.com", To: "carol@w.com", Subject: "", Date: at(3)},
		{UID: 4, MessageID: "<d@x>", From: "me@x.com", To: "dave@w.com", Subject: "", Date: at(4)},
	}
	threads := GroupEmailsIntoThreads(emails)
	if len(threads) != 4 {
		t.Fatalf("got %d threads, want 4", len(threads))
	}
}

// A reply chain linked by headers stays together even when subjects differ,
// and a header-less "Re:" to the same person joins via subject+participants.
func TestThreadsReplyChain(t *testing.T) {
	emails := []*Email{
		{UID: 1, MessageID: "root@x", From: "bob@y.com", To: "me@x.com", Subject: "Teklif", Date: at(1)},
		{UID: 2, MessageID: "r1@x", From: "me@x.com", To: "Bob <BOB@y.com>", Subject: "Re: Teklif (güncel)", InReplyTo: "root@x", References: "<root@x>", Date: at(2)},
		{UID: 3, MessageID: "r2@x", From: "bob@y.com", To: "me@x.com", Subject: "RE: Teklif", Date: at(3)},
		{UID: 4, MessageID: "other@x", From: "alice@z.com", To: "me@x.com", Subject: "Re: Teklif", Date: at(4)},
	}
	threads := GroupEmailsIntoThreads(emails)
	th := threadOf(threads, 1)
	if th == nil || th.Count != 3 || th.ThreadID != "root@x" {
		t.Fatalf("thread of root = %+v, want 3 messages with id root@x", th)
	}
	if threadOf(threads, 4) == th {
		t.Fatal("alice's mail merged into bob's conversation")
	}
}

// In Sent the original lives in INBOX; replies still group through References.
func TestThreadsRepliesWithoutRoot(t *testing.T) {
	emails := []*Email{
		{UID: 1, MessageID: "r1@x", From: "me@x.com", To: "bob@y.com", Subject: "Re: A", References: "<root@x>", Date: at(1)},
		{UID: 2, MessageID: "r2@x", From: "me@x.com", To: "bob@y.com", Subject: "Re: A", References: "<root@x> <r1@x> <bobreply@y>", Date: at(2)},
	}
	if threads := GroupEmailsIntoThreads(emails); len(threads) != 1 || threads[0].ThreadID != "r1@x" {
		t.Fatalf("got %d threads (%+v), want 1 with id r1@x", len(threads), threads)
	}
}
