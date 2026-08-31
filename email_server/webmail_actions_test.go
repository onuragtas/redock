package email_server

import (
	"strings"
	"testing"
	"time"
)

// deliverProbe puts one message in a folder and returns its UID.
func deliverProbe(t *testing.T, m *EmailManager, mailboxID uint, folder, raw string) uint32 {
	t.Helper()

	account, err := m.accountForID(mailboxID)
	if err != nil {
		t.Fatalf("accountForID: %v", err)
	}
	uid, err := m.store().Deliver(account.Base, folder, []byte(raw), nil, time.Now())
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	return uid
}

func TestSetMessageFlagMarksReadAndStarred(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, "Subject: unread\r\n\r\nbody")

	emails, _ := m.WebmailMessages(mailbox.ID, inboxName, 10)
	if len(emails) != 1 || emails[0].Seen {
		t.Fatalf("a freshly delivered message must start unread: %+v", emails)
	}

	if err := m.SetMessageFlag(mailbox.ID, inboxName, uid, "seen", true); err != nil {
		t.Fatalf("SetMessageFlag: %v", err)
	}
	if err := m.SetMessageFlag(mailbox.ID, inboxName, uid, "starred", true); err != nil {
		t.Fatalf("SetMessageFlag(starred): %v", err)
	}

	emails, _ = m.WebmailMessages(mailbox.ID, inboxName, 10)
	if !emails[0].Seen {
		t.Error("the message was not marked read")
	}
	if !emails[0].Flagged {
		t.Error("the message was not starred")
	}

	// And it can be turned back off.
	if err := m.SetMessageFlag(mailbox.ID, inboxName, uid, "read", false); err != nil {
		t.Fatalf("clearing the flag: %v", err)
	}
	emails, _ = m.WebmailMessages(mailbox.ID, inboxName, 10)
	if emails[0].Seen {
		t.Error("the read flag was not cleared")
	}
}

func TestSetMessageFlagRefusesUnknownFlags(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, "Subject: x\r\n\r\nbody")

	if err := m.SetMessageFlag(mailbox.ID, inboxName, uid, "not-a-flag", true); err == nil {
		t.Fatal("an unknown flag must be refused rather than written to the filename")
	}
}

func TestDeleteMovesToTrashThenRemoves(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, "Subject: bye\r\n\r\nbody")

	if err := m.DeleteMessage(mailbox.ID, inboxName, uid); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	inbox, _ := m.WebmailMessages(mailbox.ID, inboxName, 10)
	if len(inbox) != 0 {
		t.Fatalf("the message should have left the inbox, %d remain", len(inbox))
	}
	trash, _ := m.WebmailMessages(mailbox.ID, "Trash", 10)
	if len(trash) != 1 {
		t.Fatalf("the message should be in Trash, found %d", len(trash))
	}

	// Deleting from Trash is permanent.
	if err := m.DeleteMessage(mailbox.ID, "Trash", trash[0].UID); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	trash, _ = m.WebmailMessages(mailbox.ID, "Trash", 10)
	if len(trash) != 0 {
		t.Fatal("deleting from Trash must remove the message for good")
	}
}

func TestMoveMessageBetweenFolders(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, "Subject: filed\r\n\r\nbody")

	if err := m.MoveMessage(mailbox.ID, inboxName, "Archive", uid); err != nil {
		t.Fatalf("MoveMessage: %v", err)
	}

	if inbox, _ := m.WebmailMessages(mailbox.ID, inboxName, 10); len(inbox) != 0 {
		t.Error("the message did not leave the inbox")
	}
	archive, _ := m.WebmailMessages(mailbox.ID, "Archive", 10)
	if len(archive) != 1 || archive[0].Subject != "filed" {
		t.Errorf("the message did not arrive in Archive: %+v", archive)
	}
}

const messageWithAttachment = "From: alice@example.com\r\n" +
	"To: bob@example.net\r\n" +
	"Subject: report\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"BOUND\"\r\n" +
	"\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"see attached\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/csv; name=\"figures.csv\"\r\n" +
	"Content-Disposition: attachment; filename=\"figures.csv\"\r\n" +
	"\r\n" +
	"a,b,c\r\n1,2,3\r\n" +
	"--BOUND--\r\n"

func TestAttachmentsAreListedAndDownloadable(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, messageWithAttachment)

	list, err := m.ListAttachments(mailbox.ID, inboxName, uid)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly the one attachment, got %d: %+v", len(list), list)
	}
	if list[0].Filename != "figures.csv" {
		t.Errorf("the filename was lost: %+v", list[0])
	}
	if list[0].Data != nil {
		t.Error("the listing must not carry the bytes")
	}
	if list[0].Size == 0 {
		t.Error("the listing should still report the size")
	}

	attachment, err := m.Attachment(mailbox.ID, inboxName, uid, 0)
	if err != nil {
		t.Fatalf("Attachment: %v", err)
	}
	if !strings.Contains(string(attachment.Data), "1,2,3") {
		t.Errorf("the attachment content is wrong: %q", attachment.Data)
	}

	if _, err := m.Attachment(mailbox.ID, inboxName, uid, 5); err == nil {
		t.Error("an out-of-range index must be refused")
	}
}

func TestPlainMessageHasNoAttachments(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, "Subject: plain\r\nContent-Type: text/plain\r\n\r\njust text")

	list, err := m.ListAttachments(mailbox.ID, inboxName, uid)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("a plain text message has no attachments: %+v", list)
	}
}

func TestRawMessageReturnsTheOriginal(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	uid := deliverProbe(t, m, mailbox.ID, inboxName, messageWithAttachment)

	raw, err := m.RawMessage(mailbox.ID, inboxName, uid)
	if err != nil {
		t.Fatalf("RawMessage: %v", err)
	}
	if string(raw) != messageWithAttachment {
		t.Error("the original message was not returned byte for byte")
	}
}

func TestSaveDraftLandsInDrafts(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	uid, err := m.SaveDraft(mailbox.ID, &EmailMessage{
		From:      mailbox.Email,
		To:        []string{"bob@example.net"},
		Subject:   "half written",
		BodyPlain: "to be continued",
	})
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if uid == 0 {
		t.Error("a saved draft should get a UID")
	}

	drafts, err := m.WebmailMessages(mailbox.ID, "Drafts", 10)
	if err != nil {
		t.Fatalf("WebmailMessages: %v", err)
	}
	if len(drafts) != 1 || drafts[0].Subject != "half written" {
		t.Fatalf("the draft was not stored: %+v", drafts)
	}
	if !drafts[0].Draft {
		t.Error("a draft must carry the \\Draft flag")
	}
}
