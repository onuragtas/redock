package email_server

import (
	"encoding/json"
	"testing"

	"redock/platform/memory"
)

func addFilter(t *testing.T, m *EmailManager, mailboxID uint, name string, priority int,
	matchAll bool, conditions []FilterCondition, actions []FilterAction) *EmailFilter {
	t.Helper()

	conditionJSON, _ := json.Marshal(conditions)
	actionJSON, _ := json.Marshal(actions)

	filter, err := m.AddFilter(&EmailFilter{
		MailboxID:  mailboxID,
		Name:       name,
		Priority:   priority,
		Enabled:    true,
		MatchAll:   matchAll,
		Conditions: string(conditionJSON),
		Actions:    string(actionJSON),
	})
	if err != nil {
		t.Fatalf("AddFilter(%s): %v", name, err)
	}
	return filter
}

const newsletter = "From: news@example.net\r\n" +
	"To: alice@example.com\r\n" +
	"Subject: Weekly digest\r\n" +
	"\r\n" +
	"the news\r\n"

func TestFilterFilesMessageIntoAFolder(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	addFilter(t, m, mailbox.ID, "newsletters", 10, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}, {Type: ActionMarkRead}})

	account := m.LookupAccount(mailbox.Email)
	outcome := m.applyFilters(account, []byte(newsletter), inboxName)

	if outcome.Folder != "Archive" {
		t.Errorf("the message should have been filed in Archive, got %q", outcome.Folder)
	}
	if !hasFlag(outcome.Flags, imapFlagSeen) {
		t.Error("the mark-read action did not apply")
	}
	if len(outcome.Applied) != 1 || outcome.Applied[0] != "newsletters" {
		t.Errorf("the applied rule was not recorded: %v", outcome.Applied)
	}
}

func TestFilterLeavesUnmatchedMailAlone(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	addFilter(t, m, mailbox.ID, "newsletters", 10, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}})

	account := m.LookupAccount(mailbox.Email)
	personal := []byte("From: bob@example.net\r\nSubject: lunch?\r\n\r\nbody")

	outcome := m.applyFilters(account, personal, inboxName)
	if outcome.Folder != inboxName {
		t.Errorf("an unmatched message must stay in the inbox, got %q", outcome.Folder)
	}
	if len(outcome.Applied) != 0 {
		t.Errorf("no rule should have matched: %v", outcome.Applied)
	}
}

func TestFilterMatchAllRequiresEveryCondition(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	addFilter(t, m, mailbox.ID, "strict", 10, true,
		[]FilterCondition{
			{Field: FieldFrom, Operator: OpContains, Value: "news@"},
			{Field: FieldSubject, Operator: OpContains, Value: "invoice"},
		},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}})

	account := m.LookupAccount(mailbox.Email)
	if outcome := m.applyFilters(account, []byte(newsletter), inboxName); outcome.Folder != inboxName {
		t.Errorf("match-all should not fire when only one condition holds: %+v", outcome)
	}

	// Any-match fires on the first condition.
	_ = memory.Delete[*EmailFilter](m.db, "email_filters", m.ListFilters(mailbox.ID)[0].ID)
	addFilter(t, m, mailbox.ID, "loose", 10, false,
		[]FilterCondition{
			{Field: FieldFrom, Operator: OpContains, Value: "news@"},
			{Field: FieldSubject, Operator: OpContains, Value: "invoice"},
		},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}})

	if outcome := m.applyFilters(account, []byte(newsletter), inboxName); outcome.Folder != "Archive" {
		t.Errorf("match-any should fire on one condition: %+v", outcome)
	}
}

func TestFiltersRunInPriorityOrderAndStop(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	// The lower priority number runs first and stops the chain.
	addFilter(t, m, mailbox.ID, "first", 1, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}, {Type: ActionStop}})
	addFilter(t, m, mailbox.ID, "second", 5, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Junk"}})

	account := m.LookupAccount(mailbox.Email)
	outcome := m.applyFilters(account, []byte(newsletter), inboxName)

	if outcome.Folder != "Archive" {
		t.Errorf("the stop action should have prevented the second rule: %+v", outcome)
	}
	if len(outcome.Applied) != 1 {
		t.Errorf("only the first rule should have run: %v", outcome.Applied)
	}
}

func TestFilterCanDiscardAMessage(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	addFilter(t, m, mailbox.ID, "nuisance", 10, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionDelete}})

	account := m.LookupAccount(mailbox.Email)
	if outcome := m.applyFilters(account, []byte(newsletter), inboxName); !outcome.Discard {
		t.Errorf("the delete action did not discard the message: %+v", outcome)
	}
}

func TestDisabledFilterDoesNotRun(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	filter := addFilter(t, m, mailbox.ID, "off", 10, true,
		[]FilterCondition{{Field: FieldFrom, Operator: OpContains, Value: "news@"}},
		[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}})

	filter.Enabled = false
	if err := memory.Update(m.db, "email_filters", filter); err != nil {
		t.Fatalf("update filter: %v", err)
	}

	account := m.LookupAccount(mailbox.Email)
	if outcome := m.applyFilters(account, []byte(newsletter), inboxName); outcome.Folder != inboxName {
		t.Errorf("a disabled filter must not run: %+v", outcome)
	}
}

func TestFilterValidationRejectsBrokenRules(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	cases := []struct {
		name   string
		filter *EmailFilter
	}{
		{"no name", &EmailFilter{MailboxID: mailbox.ID, Conditions: `[{"field":"from","value":"x"}]`, Actions: `[{"type":"mark_read"}]`}},
		{"no conditions", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Actions: `[{"type":"mark_read"}]`}},
		{"no actions", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Conditions: `[{"field":"from","value":"y"}]`}},
		{"unknown field", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Conditions: `[{"field":"nope","value":"y"}]`, Actions: `[{"type":"mark_read"}]`}},
		{"unknown action", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Conditions: `[{"field":"from","value":"y"}]`, Actions: `[{"type":"explode"}]`}},
		{"move without folder", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Conditions: `[{"field":"from","value":"y"}]`, Actions: `[{"type":"move_to"}]`}},
		{"broken json", &EmailFilter{MailboxID: mailbox.ID, Name: "x", Conditions: `not json`, Actions: `[{"type":"mark_read"}]`}},
		{"unknown mailbox", &EmailFilter{MailboxID: 9999, Name: "x", Conditions: `[{"field":"from","value":"y"}]`, Actions: `[{"type":"mark_read"}]`}},
	}

	for _, tc := range cases {
		if _, err := m.AddFilter(tc.filter); err == nil {
			t.Errorf("%s: a broken rule was accepted and would fail silently on every message", tc.name)
		}
	}
}

func TestFilterMatchesHeadersAndBody(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")

	message := []byte("From: sender@example.net\r\n" +
		"Subject: quarterly report\r\n" +
		"X-Campaign: spring\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"the numbers are inside\r\n")

	account := m.LookupAccount(mailbox.Email)

	checks := []struct {
		name      string
		condition FilterCondition
		want      bool
	}{
		{"subject contains", FilterCondition{Field: FieldSubject, Operator: OpContains, Value: "quarterly"}, true},
		{"subject starts with", FilterCondition{Field: FieldSubject, Operator: OpStartsWith, Value: "quarterly"}, true},
		{"subject ends with", FilterCondition{Field: FieldSubject, Operator: OpEndsWith, Value: "report"}, true},
		{"from not contains", FilterCondition{Field: FieldFrom, Operator: OpNotContain, Value: "spam"}, true},
		{"custom header", FilterCondition{Field: FieldHeader, Header: "X-Campaign", Operator: OpEquals, Value: "spring"}, true},
		{"body contains", FilterCondition{Field: FieldBody, Operator: OpContains, Value: "numbers"}, true},
		{"no match", FilterCondition{Field: FieldSubject, Operator: OpContains, Value: "invoice"}, false},
	}

	for _, check := range checks {
		for _, filter := range m.ListFilters(mailbox.ID) {
			_ = m.DeleteFilter(filter.ID)
		}
		addFilter(t, m, mailbox.ID, check.name, 10, true,
			[]FilterCondition{check.condition},
			[]FilterAction{{Type: ActionMoveTo, Folder: "Archive"}})

		outcome := m.applyFilters(account, message, inboxName)
		matched := outcome.Folder == "Archive"
		if matched != check.want {
			t.Errorf("%s: matched=%v, want %v", check.name, matched, check.want)
		}
	}
}
