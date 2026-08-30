package email_server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*MaildirStore, string) {
	t.Helper()

	root := t.TempDir()
	store := NewMaildirStore(root)
	if err := store.EnsureMailbox("example.com", "alice"); err != nil {
		t.Fatalf("EnsureMailbox: %v", err)
	}
	return store, store.MailboxPath("example.com", "alice")
}

func TestEnsureMailboxCreatesMaildirLayout(t *testing.T) {
	store, base := newTestStore(t)

	for _, sub := range []string{"cur", "new", "tmp"} {
		if info, err := os.Stat(filepath.Join(base, sub)); err != nil || !info.IsDir() {
			t.Fatalf("INBOX is missing %s/", sub)
		}
	}
	for _, folder := range DefaultFolders {
		dir := folderDir(base, folder)
		if info, err := os.Stat(filepath.Join(dir, "cur")); err != nil || !info.IsDir() {
			t.Fatalf("default folder %s was not created", folder)
		}
	}

	folders, err := store.ListFolders(base)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if folders[0] != inboxName {
		t.Fatalf("expected INBOX first, got %v", folders)
	}
	if len(folders) != len(DefaultFolders)+1 {
		t.Fatalf("expected %d folders, got %v", len(DefaultFolders)+1, folders)
	}
}

func TestDeliverAssignsIncreasingUIDs(t *testing.T) {
	store, base := newTestStore(t)

	uid1, err := store.Deliver(base, inboxName, []byte("Subject: one\r\n\r\nbody"), nil, time.Now())
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	uid2, err := store.Deliver(base, inboxName, []byte("Subject: two\r\n\r\nbody"), nil, time.Now())
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if uid1 != 1 || uid2 != 2 {
		t.Fatalf("expected UIDs 1 and 2, got %d and %d", uid1, uid2)
	}

	messages, err := store.List(base, inboxName)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	// A message delivered with no flags is "new", hence \Recent and unseen.
	if !hasFlag(messages[0].Flags, imapFlagRecent) {
		t.Errorf("a freshly delivered message must be \\Recent: %v", messages[0].Flags)
	}
	if hasFlag(messages[0].Flags, imapFlagSeen) {
		t.Errorf("a freshly delivered message must not be \\Seen")
	}
}

func TestUIDsSurviveFlagChangesAndRestarts(t *testing.T) {
	store, base := newTestStore(t)

	uid, err := store.Deliver(base, inboxName, []byte("Subject: keep\r\n\r\nbody"), nil, time.Now())
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	messages, _ := store.List(base, inboxName)
	updated, err := store.SetFlags(base, inboxName, messages[0], []string{imapFlagSeen})
	if err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
	if updated.Dir != "cur" {
		t.Errorf("a flagged message must live in cur/, got %s", updated.Dir)
	}

	// A fresh store (as after a restart) must report the same UID.
	reopened := NewMaildirStore(store.Root())
	messages, err = reopened.List(base, inboxName)
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(messages) != 1 || messages[0].UID != uid {
		t.Fatalf("UID changed across restart: want %d, got %+v", uid, messages)
	}
	if !hasFlag(messages[0].Flags, imapFlagSeen) {
		t.Errorf("\\Seen was lost across restart: %v", messages[0].Flags)
	}
}

func TestFlagEncodingRoundTrip(t *testing.T) {
	flags := []string{imapFlagSeen, imapFlagAnswered, imapFlagFlagged, imapFlagDraft, imapFlagDeleted}

	letters := encodeMaildirFlags(flags)
	if letters != "DFRST" {
		t.Fatalf("maildir flag letters must be in ASCII order, got %q", letters)
	}

	decoded := decodeMaildirFlags(letters)
	for _, flag := range flags {
		if !hasFlag(decoded, flag) {
			t.Errorf("flag %s did not survive the round trip: %v", flag, decoded)
		}
	}
}

func TestMoveAndCopyBetweenFolders(t *testing.T) {
	store, base := newTestStore(t)

	if _, err := store.Deliver(base, inboxName, []byte("Subject: move me\r\n\r\nbody"), []string{imapFlagSeen}, time.Now()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	messages, _ := store.List(base, inboxName)

	if _, err := store.Copy(base, inboxName, "Archive", messages[0]); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	archived, _ := store.List(base, "Archive")
	if len(archived) != 1 {
		t.Fatalf("expected the copy in Archive, got %d messages", len(archived))
	}
	if len(mustList(t, store, base, inboxName)) != 1 {
		t.Fatal("Copy must leave the original in place")
	}

	if _, err := store.Move(base, inboxName, "Trash", messages[0]); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(mustList(t, store, base, inboxName)) != 0 {
		t.Fatal("Move must remove the original")
	}
	trashed := mustList(t, store, base, "Trash")
	if len(trashed) != 1 {
		t.Fatalf("expected the message in Trash, got %d", len(trashed))
	}
	if !hasFlag(trashed[0].Flags, imapFlagSeen) {
		t.Errorf("Move must preserve flags, got %v", trashed[0].Flags)
	}
}

func mustList(t *testing.T, store *MaildirStore, base, folder string) []MaildirMessage {
	t.Helper()
	messages, err := store.List(base, folder)
	if err != nil {
		t.Fatalf("List(%s): %v", folder, err)
	}
	return messages
}

func TestStatsCountsUnseenAndRecent(t *testing.T) {
	store, base := newTestStore(t)

	_, _ = store.Deliver(base, inboxName, []byte("Subject: a\r\n\r\nbody"), nil, time.Now())
	_, _ = store.Deliver(base, inboxName, []byte("Subject: b\r\n\r\nbody"), []string{imapFlagSeen}, time.Now())

	stats, err := store.Stats(base, inboxName)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Messages != 2 {
		t.Errorf("expected 2 messages, got %d", stats.Messages)
	}
	if stats.Unseen != 1 {
		t.Errorf("expected 1 unseen message, got %d", stats.Unseen)
	}
	if stats.Recent != 1 {
		t.Errorf("expected 1 recent message, got %d", stats.Recent)
	}
	if stats.UIDNext != 3 {
		t.Errorf("expected UIDNEXT 3, got %d", stats.UIDNext)
	}
	if stats.UIDValidity == 0 {
		t.Error("UIDVALIDITY must not be zero")
	}
}

func TestFolderCreateRenameDelete(t *testing.T) {
	store, base := newTestStore(t)

	if err := store.CreateFolder(base, "Projects/Redock"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	// Nested IMAP names become Maildir++ dotted directories.
	if _, err := os.Stat(filepath.Join(base, ".Projects.Redock", "cur")); err != nil {
		t.Fatalf("nested folder was not created as a Maildir++ directory: %v", err)
	}

	folders, _ := store.ListFolders(base)
	if !containsString(folders, "Projects/Redock") {
		t.Fatalf("nested folder missing from listing: %v", folders)
	}

	if err := store.RenameFolder(base, "Projects/Redock", "Work"); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	folders, _ = store.ListFolders(base)
	if containsString(folders, "Projects/Redock") || !containsString(folders, "Work") {
		t.Fatalf("rename did not take effect: %v", folders)
	}

	if err := store.DeleteFolder(base, "Work"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if err := store.DeleteFolder(base, inboxName); err == nil {
		t.Fatal("deleting INBOX must be refused")
	}
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func TestRemoveDeletesMessage(t *testing.T) {
	store, base := newTestStore(t)

	_, _ = store.Deliver(base, inboxName, []byte("Subject: bye\r\n\r\nbody"), nil, time.Now())
	messages := mustList(t, store, base, inboxName)

	if err := store.Remove(base, inboxName, messages[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(mustList(t, store, base, inboxName)) != 0 {
		t.Fatal("message was not removed")
	}
}

func TestReadReturnsTheStoredBytes(t *testing.T) {
	store, base := newTestStore(t)

	body := []byte("Subject: hello\r\n\r\nthe body")
	if _, err := store.Deliver(base, inboxName, body, nil, time.Now()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	messages := mustList(t, store, base, inboxName)
	got, err := store.Read(base, inboxName, messages[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("stored bytes differ:\nwant %q\ngot  %q", body, got)
	}
}
