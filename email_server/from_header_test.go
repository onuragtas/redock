package email_server

import (
	"strings"
	"testing"
)

// A From header carrying only a bare address is one of the things a spam
// filter counts against a message. The mailbox already knows the person's
// name, so there is no reason to leave it out.
func TestFromHeaderCarriesTheDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		display string
		address string
		want    string
	}{
		{"a plain name", "Onur", "onur@example.com", `"Onur" <onur@example.com>`},
		// A comma would otherwise split the header into two addresses.
		{"a name with a comma", "Doe, Jane", "jane@example.com", `"Doe, Jane" <jane@example.com>`},
		// Non-ASCII has to be encoded, not sent raw.
		{"a name with Turkish letters", "Onur Ağtaş", "onur@example.com", "=?utf-8?q?"},
		{"no name at all", "", "onur@example.com", "onur@example.com"},
		{"only whitespace", "   ", "onur@example.com", "onur@example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatFrom(tc.display, tc.address)
			if !strings.Contains(got, tc.want) {
				t.Errorf("formatFrom(%q, %q) = %q, want it to contain %q", tc.display, tc.address, got, tc.want)
			}
			// Whatever the display name, the address must survive intact.
			if !strings.Contains(got, tc.address) {
				t.Errorf("formatFrom(%q, %q) = %q, lost the address", tc.display, tc.address, got)
			}
		})
	}
}

// The header is for people; the envelope is for servers. A display name must
// never leak into the address used for routing or SPF.
func TestTheEnvelopeKeepsTheBareAddress(t *testing.T) {
	header := formatFrom("Onur Ağtaş", "onur@example.com")
	if got := normalizeAddress(header); got != "onur@example.com" {
		t.Errorf("normalizeAddress(%q) = %q, want the bare address", header, got)
	}
}

// The built message must carry the name in From while staying parseable.
func TestBuiltMessageHasANamedFrom(t *testing.T) {
	m := newTestManager(t)
	client := NewSMTPClient(m)

	raw, err := client.buildMIMEMessage(&EmailMessage{
		From: "onur@example.com", FromName: "Onur Ağtaş",
		To: []string{"someone@example.com"}, Subject: "hello", BodyPlain: "hi",
	})
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}

	from := headerValue(raw, "From")
	if !strings.Contains(from, "onur@example.com") {
		t.Fatalf("From = %q, want it to hold the address", from)
	}
	if from == "onur@example.com" {
		t.Error("From is still a bare address; the display name was dropped")
	}
}
