package email_server

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"redock/platform/memory"
)

// Filters are the mailbox owner's own sorting rules, applied as mail arrives:
// file newsletters into a folder, star anything from the boss, drop a known
// nuisance. The EmailFilter table has existed since the beginning; this is the
// engine that finally reads it.

// FilterField is what a condition looks at.
const (
	FieldFrom    = "from"
	FieldTo      = "to"
	FieldCc      = "cc"
	FieldSubject = "subject"
	FieldBody    = "body"
	FieldHeader  = "header"
)

// FilterOperator is how a condition compares.
const (
	OpContains   = "contains"
	OpNotContain = "not_contains"
	OpEquals     = "equals"
	OpStartsWith = "starts_with"
	OpEndsWith   = "ends_with"
)

// FilterActionType is what a matching rule does.
const (
	ActionMoveTo   = "move_to"
	ActionMarkRead = "mark_read"
	ActionStar     = "star"
	ActionDelete   = "delete"
	ActionStop     = "stop"
)

// FilterCondition is one test against a message.
type FilterCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	// Header names the header to read when Field is "header".
	Header string `json:"header,omitempty"`
}

// FilterAction is one thing to do with a matching message.
type FilterAction struct {
	Type string `json:"type"`
	// Folder is the destination for move_to.
	Folder string `json:"folder,omitempty"`
}

// filterOutcome is what the rules decided for one message.
type filterOutcome struct {
	Folder  string
	Flags   []string
	Discard bool
	Applied []string // names of the rules that matched, for the log
}

// applyFilters runs a mailbox's rules over an arriving message and returns
// where it should go. The default is the folder the caller chose.
func (m *EmailManager) applyFilters(account *Account, raw []byte, defaultFolder string) filterOutcome {
	outcome := filterOutcome{Folder: defaultFolder}
	if m.db == nil || account.Mailbox == nil {
		return outcome
	}

	filters := memory.Filter[*EmailFilter](m.db, "email_filters", func(f *EmailFilter) bool {
		return f != nil && !f.IsDeleted() && f.Enabled && f.MailboxID == account.Mailbox.ID
	})
	if len(filters) == 0 {
		return outcome
	}

	// Lower priority numbers run first, the way a rule list reads top to bottom.
	sort.SliceStable(filters, func(i, j int) bool { return filters[i].Priority < filters[j].Priority })

	for _, filter := range filters {
		conditions, err := parseConditions(filter.Conditions)
		if err != nil {
			log.Printf("mail: filter %q has unreadable conditions: %v", filter.Name, err)
			continue
		}
		if !matchesConditions(raw, conditions, filter.MatchAll) {
			continue
		}

		actions, err := parseActions(filter.Actions)
		if err != nil {
			log.Printf("mail: filter %q has unreadable actions: %v", filter.Name, err)
			continue
		}

		outcome.Applied = append(outcome.Applied, filter.Name)
		stop := false
		for _, action := range actions {
			switch action.Type {
			case ActionMoveTo:
				if action.Folder != "" {
					outcome.Folder = action.Folder
				}
			case ActionMarkRead:
				if !hasFlag(outcome.Flags, imapFlagSeen) {
					outcome.Flags = append(outcome.Flags, imapFlagSeen)
				}
			case ActionStar:
				if !hasFlag(outcome.Flags, imapFlagFlagged) {
					outcome.Flags = append(outcome.Flags, imapFlagFlagged)
				}
			case ActionDelete:
				outcome.Discard = true
				stop = true
			case ActionStop:
				stop = true
			}
		}
		if stop {
			break
		}
	}

	return outcome
}

// parseConditions reads the stored JSON. A single condition object is accepted
// as well as a list, because that is what hand-written rules tend to look like.
func parseConditions(stored string) ([]FilterCondition, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return nil, nil
	}

	var list []FilterCondition
	if err := json.Unmarshal([]byte(stored), &list); err == nil {
		return list, nil
	}

	var single FilterCondition
	if err := json.Unmarshal([]byte(stored), &single); err != nil {
		return nil, err
	}
	return []FilterCondition{single}, nil
}

func parseActions(stored string) ([]FilterAction, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return nil, nil
	}

	var list []FilterAction
	if err := json.Unmarshal([]byte(stored), &list); err == nil {
		return list, nil
	}

	var single FilterAction
	if err := json.Unmarshal([]byte(stored), &single); err != nil {
		return nil, err
	}
	return []FilterAction{single}, nil
}

// matchesConditions evaluates a rule against a message.
func matchesConditions(raw []byte, conditions []FilterCondition, matchAll bool) bool {
	if len(conditions) == 0 {
		return false // a rule with no test must never match everything
	}

	for _, condition := range conditions {
		matched := matchesCondition(raw, condition)
		if matchAll && !matched {
			return false
		}
		if !matchAll && matched {
			return true
		}
	}
	return matchAll
}

func matchesCondition(raw []byte, condition FilterCondition) bool {
	subject := ""
	value := strings.ToLower(strings.TrimSpace(condition.Value))
	if value == "" {
		return false
	}

	var field string
	switch strings.ToLower(condition.Field) {
	case FieldFrom:
		field = headerValue(raw, "From")
	case FieldTo:
		field = headerValue(raw, "To")
	case FieldCc:
		field = headerValue(raw, "Cc")
	case FieldSubject:
		subject = headerValue(raw, "Subject")
		field = subject
	case FieldBody:
		plain, html, _, _ := extractBodyFromRawMessage(raw)
		field = plain + " " + html
	case FieldHeader:
		if condition.Header == "" {
			return false
		}
		field = headerValue(raw, condition.Header)
	default:
		return false
	}

	field = strings.ToLower(field)
	switch strings.ToLower(condition.Operator) {
	case OpContains, "":
		return strings.Contains(field, value)
	case OpNotContain:
		return !strings.Contains(field, value)
	case OpEquals:
		return strings.TrimSpace(field) == value
	case OpStartsWith:
		return strings.HasPrefix(strings.TrimSpace(field), value)
	case OpEndsWith:
		return strings.HasSuffix(strings.TrimSpace(field), value)
	default:
		return false
	}
}

// ---- filter management ----

// ListFilters returns a mailbox's rules in the order they run.
func (m *EmailManager) ListFilters(mailboxID uint) []*EmailFilter {
	filters := memory.Filter[*EmailFilter](m.db, "email_filters", func(f *EmailFilter) bool {
		return f != nil && !f.IsDeleted() && f.MailboxID == mailboxID
	})
	sort.SliceStable(filters, func(i, j int) bool { return filters[i].Priority < filters[j].Priority })
	return filters
}

// AddFilter stores a rule after checking that it can actually run.
func (m *EmailManager) AddFilter(filter *EmailFilter) (*EmailFilter, error) {
	if err := m.validateFilter(filter); err != nil {
		return nil, err
	}
	if err := memory.Create(m.db, "email_filters", filter); err != nil {
		return nil, fmt.Errorf("failed to store the filter: %w", err)
	}
	return filter, nil
}

// UpdateFilter replaces a rule's definition.
func (m *EmailManager) UpdateFilter(id uint, updated *EmailFilter) (*EmailFilter, error) {
	existing, err := memory.FindByID[*EmailFilter](m.db, "email_filters", id)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("filter not found")
	}

	updated.ID = existing.ID
	updated.MailboxID = existing.MailboxID
	if err := m.validateFilter(updated); err != nil {
		return nil, err
	}

	existing.Name = updated.Name
	existing.Priority = updated.Priority
	existing.Enabled = updated.Enabled
	existing.Conditions = updated.Conditions
	existing.MatchAll = updated.MatchAll
	existing.Actions = updated.Actions

	if err := memory.Update(m.db, "email_filters", existing); err != nil {
		return nil, fmt.Errorf("failed to update the filter: %w", err)
	}
	return existing, nil
}

// DeleteFilter removes a rule.
func (m *EmailManager) DeleteFilter(id uint) error {
	if _, err := memory.FindByID[*EmailFilter](m.db, "email_filters", id); err != nil {
		return fmt.Errorf("filter not found")
	}
	return memory.Delete[*EmailFilter](m.db, "email_filters", id)
}

// validateFilter refuses a rule that could not do anything, or that would do
// something unexpected — a rule stored broken would fail silently on every
// message that arrives.
func (m *EmailManager) validateFilter(filter *EmailFilter) error {
	if strings.TrimSpace(filter.Name) == "" {
		return fmt.Errorf("the filter needs a name")
	}
	if _, err := memory.FindByID[*EmailMailbox](m.db, "email_mailboxes", filter.MailboxID); err != nil {
		return fmt.Errorf("mailbox not found")
	}

	conditions, err := parseConditions(filter.Conditions)
	if err != nil {
		return fmt.Errorf("the conditions are not valid JSON: %w", err)
	}
	if len(conditions) == 0 {
		return fmt.Errorf("the filter needs at least one condition")
	}
	for _, condition := range conditions {
		switch strings.ToLower(condition.Field) {
		case FieldFrom, FieldTo, FieldCc, FieldSubject, FieldBody:
		case FieldHeader:
			if condition.Header == "" {
				return fmt.Errorf("a header condition must name the header")
			}
		default:
			return fmt.Errorf("unknown condition field %q", condition.Field)
		}
		if strings.TrimSpace(condition.Value) == "" {
			return fmt.Errorf("a condition needs a value to compare against")
		}
	}

	actions, err := parseActions(filter.Actions)
	if err != nil {
		return fmt.Errorf("the actions are not valid JSON: %w", err)
	}
	if len(actions) == 0 {
		return fmt.Errorf("the filter needs at least one action")
	}
	for _, action := range actions {
		switch action.Type {
		case ActionMarkRead, ActionStar, ActionDelete, ActionStop:
		case ActionMoveTo:
			if strings.TrimSpace(action.Folder) == "" {
				return fmt.Errorf("a move action must name a folder")
			}
		default:
			return fmt.Errorf("unknown action %q", action.Type)
		}
	}
	return nil
}
