package controllers

import (
	"testing"

	"redock/email_server"
)

// The in-memory database hands out pointers to the live records. Clearing a
// secret on one of those pointers to hide it from an API response does not just
// hide it — it deletes it, and the periodic flush writes the loss to disk. That
// is how every mailbox password hash was wiped simply by listing mailboxes.
func TestSanitizedMailboxDoesNotTouchTheStoredRecord(t *testing.T) {
	stored := &email_server.EmailMailbox{
		Email:         "alice@example.com",
		Username:      "alice",
		Password:      "$2a$10$abcdefghijklmnopqrstuv",
		PlainPassword: "encrypted-blob",
		Name:          "Alice",
	}

	response := sanitizedMailbox(stored)

	if response.Password != "" || response.PlainPassword != "" {
		t.Errorf("the response still carries secrets: %+v", response)
	}
	if stored.Password == "" {
		t.Fatal("the stored password hash was destroyed — authentication would break")
	}
	if stored.PlainPassword == "" {
		t.Fatal("the stored encrypted password was destroyed")
	}
	if response == stored {
		t.Fatal("the response must be a copy, not the stored record")
	}

	// Everything else must survive the copy.
	if response.Email != stored.Email || response.Username != stored.Username || response.Name != stored.Name {
		t.Errorf("the copy lost non-secret fields: %+v", response)
	}
}

func TestSanitizedMailboxHandlesNil(t *testing.T) {
	if sanitizedMailbox(nil) != nil {
		t.Error("a nil mailbox should sanitise to nil")
	}
}
