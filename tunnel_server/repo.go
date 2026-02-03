package tunnel_server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"redock/platform/database"
	"redock/platform/memory"
	"strings"
)

// normalizeBaseURL trims and removes trailing slash so credential lookup matches regardless of URL form.
func normalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, "/")
}

// GetDB returns the global memory DB (must be initialized before tunnel_server use).
func GetDB() *memory.Database {
	return database.GetMemoryDB()
}

// CreateDomain creates a new TunnelDomain.
func CreateDomain(d *TunnelDomain) error {
	return memory.Create[*TunnelDomain](GetDB(), TableTunnelDomains, d)
}

// UpdateDomain updates an existing TunnelDomain.
func UpdateDomain(d *TunnelDomain) error {
	return memory.Update[*TunnelDomain](GetDB(), TableTunnelDomains, d)
}

// DeleteDomainByID deletes a TunnelDomain by ID.
func DeleteDomainByID(id uint) error {
	return memory.Delete[*TunnelDomain](GetDB(), TableTunnelDomains, id)
}

// FindDomainByID finds a TunnelDomain by ID.
func FindDomainByID(id uint) (*TunnelDomain, error) {
	return memory.FindByID[*TunnelDomain](GetDB(), TableTunnelDomains, id)
}

// FindDomainsByUserID returns all domains for a tunnel user.
func FindDomainsByUserID(userID uint) []*TunnelDomain {
	return memory.Where[*TunnelDomain](GetDB(), TableTunnelDomains, "UserID", userID)
}

// AllDomains returns all tunnel domains.
func AllDomains() []*TunnelDomain {
	return memory.FindAll[*TunnelDomain](GetDB(), TableTunnelDomains)
}

// FindDomainBySubdomain finds a domain by subdomain (within same suffix; caller should normalize).
func FindDomainBySubdomain(subdomain string) *TunnelDomain {
	all := memory.FindAll[*TunnelDomain](GetDB(), TableTunnelDomains)
	for _, d := range all {
		if d.Subdomain == subdomain {
			return d
		}
	}
	return nil
}

// GenerateRandomSubdomain returns a random subdomain (e.g. "a1b2c3d4") that does not already exist. Max 20 attempts.
func GenerateRandomSubdomain() (string, error) {
	const length = 8
	const maxAttempts = 20
	b := make([]byte, length/2)
	for i := 0; i < maxAttempts; i++ {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		sub := hex.EncodeToString(b)
		if FindDomainBySubdomain(sub) == nil {
			return sub, nil
		}
	}
	return "", fmt.Errorf("could not generate unique subdomain after %d attempts", maxAttempts)
}

// FindDomainByFullDomain finds a domain by full domain (e.g. myapp.example.com).
func FindDomainByFullDomain(fullDomain string) *TunnelDomain {
	all := memory.FindAll[*TunnelDomain](GetDB(), TableTunnelDomains)
	for _, d := range all {
		if d.FullDomain == fullDomain {
			return d
		}
	}
	return nil
}

// NextPortForDomain returns the next port using current domains.
func NextPortForDomain() (int, error) {
	return NextPort(func() []*TunnelDomain { return AllDomains() })
}

// CreateTunnelUser creates a new TunnelUser.
func CreateTunnelUser(u *TunnelUser) error {
	return memory.Create[*TunnelUser](GetDB(), TableTunnelUsers, u)
}

// FindTunnelUserByID finds a TunnelUser by ID.
func FindTunnelUserByID(id uint) (*TunnelUser, error) {
	return memory.FindByID[*TunnelUser](GetDB(), TableTunnelUsers, id)
}

// FindTunnelUserByUsername finds a TunnelUser by username.
func FindTunnelUserByUsername(username string) *TunnelUser {
	all := memory.FindAll[*TunnelUser](GetDB(), TableTunnelUsers)
	for _, u := range all {
		if u.Username == username {
			return u
		}
	}
	return nil
}

// CredentialByBaseURL returns the stored credential for a tunnel server BaseURL (any user).
// For per-user credentials, filter by UserID in caller.
func CredentialByBaseURL(baseURL string) *TunnelServerCredential {
	baseURL = normalizeBaseURL(baseURL)
	all := memory.FindAll[*TunnelServerCredential](GetDB(), TableTunnelCreds)
	for _, c := range all {
		if normalizeBaseURL(c.BaseURL) == baseURL {
			return c
		}
	}
	return nil
}

// CredentialByBaseURLAndUser returns credential for BaseURL and UserID.
func CredentialByBaseURLAndUser(baseURL string, userID uint) *TunnelServerCredential {
	baseURL = normalizeBaseURL(baseURL)
	all := memory.FindAll[*TunnelServerCredential](GetDB(), TableTunnelCreds)
	for _, c := range all {
		if normalizeBaseURL(c.BaseURL) == baseURL && c.UserID == userID {
			return c
		}
	}
	return nil
}

// SaveCredential creates or updates a credential (upsert by BaseURL + UserID).
func SaveCredential(c *TunnelServerCredential) error {
	c.BaseURL = normalizeBaseURL(c.BaseURL)
	existing := CredentialByBaseURLAndUser(c.BaseURL, c.UserID)
	if existing != nil {
		c.ID = existing.ID
		c.CreatedAt = existing.CreatedAt
		return memory.Update[*TunnelServerCredential](GetDB(), TableTunnelCreds, c)
	}
	return memory.Create[*TunnelServerCredential](GetDB(), TableTunnelCreds, c)
}

// DeleteCredentialByID deletes a credential by ID.
func DeleteCredentialByID(id uint) error {
	return memory.Delete[*TunnelServerCredential](GetDB(), TableTunnelCreds, id)
}

// --- TunnelServer (federation sunucu listesi) ---

// AllTunnelServers returns all tunnel servers, sorted by Order then ID.
func AllTunnelServers() []*TunnelServer {
	all := memory.FindAll[*TunnelServer](GetDB(), TableTunnelServers)
	// Sort by Order, then ID
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Order < all[i].Order || (all[j].Order == all[i].Order && all[j].ID < all[i].ID) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	return all
}

// CreateTunnelServer creates a new TunnelServer.
func CreateTunnelServer(s *TunnelServer) error {
	return memory.Create[*TunnelServer](GetDB(), TableTunnelServers, s)
}

// UpdateTunnelServer updates an existing TunnelServer.
func UpdateTunnelServer(s *TunnelServer) error {
	return memory.Update[*TunnelServer](GetDB(), TableTunnelServers, s)
}

// DeleteTunnelServerByID deletes a TunnelServer by ID.
func DeleteTunnelServerByID(id uint) error {
	return memory.Delete[*TunnelServer](GetDB(), TableTunnelServers, id)
}

// FindTunnelServerByID finds a TunnelServer by ID.
func FindTunnelServerByID(id uint) (*TunnelServer, error) {
	return memory.FindByID[*TunnelServer](GetDB(), TableTunnelServers, id)
}

// SetDefaultTunnelServer sets the server with the given ID as default; others are set to non-default.
func SetDefaultTunnelServer(id uint) error {
	all := memory.FindAll[*TunnelServer](GetDB(), TableTunnelServers)
	for _, s := range all {
		s.IsDefault = (s.ID == id)
		if err := memory.Update[*TunnelServer](GetDB(), TableTunnelServers, s); err != nil {
			return err
		}
	}
	return nil
}
