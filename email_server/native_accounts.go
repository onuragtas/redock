package email_server

import (
	"fmt"
	"strings"

	"redock/platform/memory"

	"golang.org/x/crypto/bcrypt"
)

// Account is a resolved local mailbox: everything the SMTP and IMAP servers
// need to accept mail for someone or let them log in. This is what replaces
// Dovecot's user database — the source of truth is the memory DB.
type Account struct {
	Mailbox *EmailMailbox
	Domain  *EmailDomain
	// Base is the account's Maildir root.
	Base string
}

// Address is the account's full email address.
func (a *Account) Address() string {
	if a.Mailbox == nil {
		return ""
	}
	return a.Mailbox.Email
}

// LookupDomain finds an enabled local domain by name.
func (m *EmailManager) LookupDomain(name string) *EmailDomain {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || m.db == nil {
		return nil
	}

	domains := memory.Filter[*EmailDomain](m.db, "email_domains", func(d *EmailDomain) bool {
		return !d.IsDeleted() && strings.EqualFold(d.Domain, name)
	})
	if len(domains) == 0 {
		return nil
	}
	return domains[0]
}

// IsLocalDomain reports whether this server is responsible for a domain.
func (m *EmailManager) IsLocalDomain(name string) bool {
	d := m.LookupDomain(name)
	return d != nil && d.Enabled
}

// LookupAccount resolves an address to a local mailbox, following aliases and
// the domain catch-all. Returns nil when the address is not local.
func (m *EmailManager) LookupAccount(address string) *Account {
	address = normalizeAddress(address)
	if address == "" || m.db == nil {
		return nil
	}

	if account := m.lookupMailbox(address); account != nil {
		return account
	}

	local, domainName := splitAddress(address)
	domain := m.LookupDomain(domainName)
	if domain == nil || !domain.Enabled {
		return nil
	}

	// Aliases: alias → destination mailbox.
	aliases := memory.Filter[*EmailAlias](m.db, "email_aliases", func(a *EmailAlias) bool {
		return !a.IsDeleted() && a.Enabled && a.DomainID == domain.ID &&
			(strings.EqualFold(a.Alias, address) || strings.EqualFold(a.Alias, local))
	})
	for _, alias := range aliases {
		if alias.DestinationID != 0 {
			if mb, err := memory.FindByID[*EmailMailbox](m.db, "email_mailboxes", alias.DestinationID); err == nil && mb != nil && !mb.IsDeleted() {
				return m.accountFor(mb)
			}
		}
		if alias.Destination != "" {
			if account := m.lookupMailbox(normalizeAddress(alias.Destination)); account != nil {
				return account
			}
		}
	}

	// Domain catch-all as the last resort.
	if domain.CatchAll != "" {
		if account := m.lookupMailbox(normalizeAddress(domain.CatchAll)); account != nil {
			return account
		}
	}

	return nil
}

func (m *EmailManager) lookupMailbox(address string) *Account {
	mailboxes := memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return !mb.IsDeleted() && strings.EqualFold(mb.Email, address)
	})
	if len(mailboxes) == 0 {
		return nil
	}
	return m.accountFor(mailboxes[0])
}

func (m *EmailManager) accountFor(mb *EmailMailbox) *Account {
	domain, err := memory.FindByID[*EmailDomain](m.db, "email_domains", mb.DomainID)
	if err != nil || domain == nil {
		// Fall back to the domain part of the address so a mailbox with a
		// dangling DomainID is still deliverable.
		_, domainName := splitAddress(mb.Email)
		domain = m.LookupDomain(domainName)
	}
	if domain == nil {
		return nil
	}

	return &Account{
		Mailbox: mb,
		Domain:  domain,
		Base:    m.store().MailboxPath(domain.Domain, mb.Username),
	}
}

// Authenticate verifies a mailbox login for SMTP submission and IMAP.
func (m *EmailManager) Authenticate(username, password string) (*Account, error) {
	account := m.LookupAccount(username)
	if account == nil || account.Mailbox == nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	if !account.Mailbox.Enabled {
		return nil, fmt.Errorf("mailbox disabled")
	}
	if account.Mailbox.Password == "" {
		return nil, fmt.Errorf("no password set for %s", username)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Mailbox.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return account, nil
}

// normalizeAddress lowercases an address and strips any display name or
// angle brackets around it.
func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if i := strings.LastIndex(address, "<"); i >= 0 {
		if j := strings.Index(address[i:], ">"); j > 0 {
			address = address[i+1 : i+j]
		}
	}
	return strings.ToLower(strings.TrimSpace(address))
}

// splitAddress splits an address into its local part and domain.
func splitAddress(address string) (local, domain string) {
	idx := strings.LastIndex(address, "@")
	if idx < 0 {
		return address, ""
	}
	return address[:idx], address[idx+1:]
}
