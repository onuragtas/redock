package email_server

import "testing"

// UIDs are handed out per folder, so the same number means a different message
// in Drafts than it does in the inbox. A delete that does not say which folder
// it means is not a delete of the message the caller had in mind: the dashboard
// once dropped the folder on the way, and deleting a draft moved an unrelated
// inbox message to Trash while the draft stayed where it was.
func TestDeleteUsesTheFolderItIsGiven(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	account := m.LookupAccount("alice@example.com")

	draftUID, err := m.SaveDraft(mailbox.ID, &EmailMessage{
		From: "alice@example.com", To: []string{"bob@example.com"}, Subject: "draft",
	})
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	if err := m.deliverLocal(account, inboxName,
		[]byte("From: someone@outside.test\r\nTo: alice@example.com\r\nSubject: keep me\r\n\r\nbody\r\n"), nil); err != nil {
		t.Fatalf("deliverLocal: %v", err)
	}

	// Both folders number their first message 1, which is what made the
	// mix-up invisible.
	inbox, _ := m.store().List(account.Base, inboxName)
	if len(inbox) != 1 || inbox[0].UID != draftUID {
		t.Fatalf("this test needs the same UID in both folders: draft=%d inbox=%v", draftUID, inbox)
	}

	if err := m.DeleteMessage(mailbox.ID, "Drafts", draftUID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	drafts, _ := m.store().List(account.Base, "Drafts")
	if len(drafts) != 0 {
		t.Errorf("the draft is still in Drafts: %d left", len(drafts))
	}

	inboxAfter, _ := m.store().List(account.Base, inboxName)
	if len(inboxAfter) != 1 {
		t.Fatalf("deleting a draft removed an inbox message: inbox has %d, want 1", len(inboxAfter))
	}

	// And the message that did move is the draft, not the inbox message.
	trash, _ := m.store().List(account.Base, "Trash")
	if len(trash) != 1 {
		t.Fatalf("Trash holds %d messages, want 1", len(trash))
	}
	raw, err := m.store().Read(account.Base, "Trash", trash[0])
	if err != nil {
		t.Fatalf("read the trashed message: %v", err)
	}
	if subject := headerValue(raw, "Subject"); subject != "draft" {
		t.Errorf("Trash holds %q, want the draft", subject)
	}
}

// A message already in Trash is deleted for good rather than moved again.
func TestDeleteFromTrashRemovesForGood(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	account := m.LookupAccount("alice@example.com")

	uid, err := m.SaveDraft(mailbox.ID, &EmailMessage{From: "alice@example.com", Subject: "draft"})
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := m.DeleteMessage(mailbox.ID, "Drafts", uid); err != nil {
		t.Fatalf("move to Trash: %v", err)
	}

	trash, _ := m.store().List(account.Base, "Trash")
	if len(trash) != 1 {
		t.Fatalf("Trash holds %d, want 1", len(trash))
	}
	if err := m.DeleteMessage(mailbox.ID, "Trash", trash[0].UID); err != nil {
		t.Fatalf("delete from Trash: %v", err)
	}
	if remaining, _ := m.store().List(account.Base, "Trash"); len(remaining) != 0 {
		t.Errorf("Trash still holds %d messages", len(remaining))
	}
}
