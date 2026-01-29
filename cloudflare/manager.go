package cloudflare

import (
	"context"
	"fmt"
	"log"
	"redock/platform/memory"
	"sync"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
)

var (
	managerInstance *CloudflareManager
	managerOnce     sync.Once
)

// CloudflareManager manages Cloudflare API operations
type CloudflareManager struct {
	db      *memory.Database
	clients map[uint]*cf.API // accountID -> API client
	mutex   sync.RWMutex
}

// GetManager returns singleton instance
func GetManager() *CloudflareManager {
	managerOnce.Do(func() {
		managerInstance = &CloudflareManager{
			clients: make(map[uint]*cf.API),
		}
	})
	return managerInstance
}

// Init initializes Cloudflare manager
func (m *CloudflareManager) Init(db *memory.Database) error {
	m.db = db
	
	// Load accounts and initialize clients
	accounts := memory.FindAll[*CloudflareAccount](db, "cloudflare_accounts")
	for _, account := range accounts {
		if account.Enabled {
			if err := m.initializeClient(account); err != nil {
				log.Printf("⚠️  Failed to initialize Cloudflare client for %s: %v", account.Name, err)
			}
		}
	}
	
	return nil
}

// initializeClient creates and caches a Cloudflare API client
func (m *CloudflareManager) initializeClient(account *CloudflareAccount) error {
	var api *cf.API
	var err error
	
	// Prefer API Token over API Key
	if account.APIToken != "" {
		api, err = cf.NewWithAPIToken(account.APIToken)
	} else if account.APIKey != "" && account.Email != "" {
		api, err = cf.New(account.APIKey, account.Email)
	} else {
		return fmt.Errorf("no valid credentials found")
	}
	
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}
	
	m.mutex.Lock()
	m.clients[account.ID] = api
	m.mutex.Unlock()
	
	return nil
}

// GetClient returns API client for an account
func (m *CloudflareManager) GetClient(accountID uint) (*cf.API, error) {
	m.mutex.RLock()
	client, exists := m.clients[accountID]
	m.mutex.RUnlock()
	
	if !exists {
		account, err := memory.FindByID[*CloudflareAccount](m.db, "cloudflare_accounts", accountID)
		if err != nil {
			return nil, fmt.Errorf("account not found: %w", err)
		}
		
		if err := m.initializeClient(account); err != nil {
			return nil, err
		}
		
		m.mutex.RLock()
		client = m.clients[accountID]
		m.mutex.RUnlock()
	}
	
	return client, nil
}

// AddAccount adds a new Cloudflare account
func (m *CloudflareManager) AddAccount(name, email, apiKey, apiToken string) (*CloudflareAccount, error) {
	// Create temporary client for testing
	var api *cf.API
	var err error
	
	if apiToken != "" {
		api, err = cf.NewWithAPIToken(apiToken)
	} else if apiKey != "" && email != "" {
		api, err = cf.New(apiKey, email)
	} else {
		return nil, fmt.Errorf("either API token or API key + email required")
	}
	
	if err != nil {
		return nil, fmt.Errorf("invalid credentials: %w", err)
	}
	
	ctx := context.Background()
	
	// Test credentials and get account ID
	accounts, _, err := api.Accounts(ctx, cf.AccountsListParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch account info: %w", err)
	}
	
	accountID := ""
	if len(accounts) > 0 {
		accountID = accounts[0].ID
	}
	
	// Create account record
	account := &CloudflareAccount{
		Name:      name,
		Email:     email,
		APIKey:    apiKey,
		APIToken:  apiToken,
		AccountID: accountID,
		Enabled:   true,
	}
	
	if err := memory.Create[*CloudflareAccount](m.db, "cloudflare_accounts", account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}
	
	// Now initialize client with correct ID
	if err := m.initializeClient(account); err != nil {
		log.Printf("⚠️  Failed to cache client for account %s: %v", name, err)
	}
	
	return account, nil
}

// RemoveAccount removes a Cloudflare account
func (m *CloudflareManager) RemoveAccount(accountID uint) error {
	m.mutex.Lock()
	delete(m.clients, accountID)
	m.mutex.Unlock()
	
	return memory.Delete[*CloudflareAccount](m.db, "cloudflare_accounts", accountID)
}

// SyncZones syncs zones from Cloudflare
func (m *CloudflareManager) SyncZones(accountID uint) error {
	client, err := m.GetClient(accountID)
	if err != nil {
		return err
	}
	
	ctx := context.Background()
	
	// List all zones
	zones, err := client.ListZones(ctx)
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}
	
	for _, zone := range zones {
		// Check if zone already exists
		existing := memory.Filter[*CloudflareZone](m.db, "cloudflare_zones", func(z *CloudflareZone) bool {
			return z.ZoneID == zone.ID
		})
		
		now := time.Now()
		if len(existing) > 0 {
			// Update existing zone
			cfZone := existing[0]
			cfZone.Name = zone.Name
			cfZone.Status = zone.Status
			cfZone.Paused = zone.Paused
			cfZone.Type = zone.Type
			cfZone.Plan = zone.Plan.Name
			cfZone.LastSync = &now
			memory.Update[*CloudflareZone](m.db, "cloudflare_zones", cfZone)
		} else {
			// Create new zone
			cfZone := &CloudflareZone{
				AccountID: accountID,
				ZoneID:    zone.ID,
				Name:      zone.Name,
				Status:    zone.Status,
				Paused:    zone.Paused,
				Type:      zone.Type,
				Plan:      zone.Plan.Name,
				LastSync:  &now,
			}
			memory.Create[*CloudflareZone](m.db, "cloudflare_zones", cfZone)
		}
	}
	
	return nil
}

// GetZone retrieves a zone by ID
func (m *CloudflareManager) GetZone(zoneID string) (*CloudflareZone, error) {
	zones := memory.Filter[*CloudflareZone](m.db, "cloudflare_zones", func(z *CloudflareZone) bool {
		return z.ZoneID == zoneID
	})
	
	if len(zones) == 0 {
		return nil, fmt.Errorf("zone not found")
	}
	
	return zones[0], nil
}

// GetZoneByName retrieves a zone by domain name
func (m *CloudflareManager) GetZoneByName(name string) (*CloudflareZone, error) {
	zones := memory.Filter[*CloudflareZone](m.db, "cloudflare_zones", func(z *CloudflareZone) bool {
		return z.Name == name
	})
	
	if len(zones) == 0 {
		return nil, fmt.Errorf("zone not found: %s", name)
	}
	
	return zones[0], nil
}

// ListZones returns all zones for an account
func (m *CloudflareManager) ListZones(accountID uint) ([]*CloudflareZone, error) {
	zones := memory.Filter[*CloudflareZone](m.db, "cloudflare_zones", func(z *CloudflareZone) bool {
		return z.AccountID == accountID
	})
	
	return zones, nil
}

// GetDB returns database connection
func (m *CloudflareManager) GetDB() (*memory.Database, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return m.db, nil
}
