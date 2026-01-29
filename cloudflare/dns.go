package cloudflare

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"redock/platform/memory"
	"strings"

	cf "github.com/cloudflare/cloudflare-go"
)

// DNSRecordParams represents parameters for creating/updating DNS records
type DNSRecordParams struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
	Proxied  *bool  `json:"proxied,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// ListDNSRecords returns all DNS records for a zone
func (m *CloudflareManager) ListDNSRecords(zoneID string) ([]*CloudflareDNSRecord, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}
	
	client, err := m.GetClient(zone.AccountID)
	if err != nil {
		return nil, err
	}
	
	ctx := context.Background()
	zoneIdentifier := cf.ZoneIdentifier(zoneID)
	
	records, _, err := client.ListDNSRecords(ctx, zoneIdentifier, cf.ListDNSRecordsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}
	
	var cfRecords []*CloudflareDNSRecord
	for _, record := range records {
		cfRecord := &CloudflareDNSRecord{
			ZoneID:   zoneID,
			RecordID: record.ID,
			Type:     record.Type,
			Name:     record.Name,
			Content:  record.Content,
			TTL:      record.TTL,
			Proxied:  *record.Proxied,
			Comment:  record.Comment,
		}
		
		if record.Priority != nil {
			cfRecord.Priority = int(*record.Priority)
		}
		
		cfRecords = append(cfRecords, cfRecord)
	}
	
	return cfRecords, nil
}

// GetDNSRecord retrieves a specific DNS record
func (m *CloudflareManager) GetDNSRecord(zoneID, recordID string) (*CloudflareDNSRecord, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}
	
	client, err := m.GetClient(zone.AccountID)
	if err != nil {
		return nil, err
	}
	
	ctx := context.Background()
	zoneIdentifier := cf.ZoneIdentifier(zoneID)
	
	record, err := client.GetDNSRecord(ctx, zoneIdentifier, recordID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS record: %w", err)
	}
	
	cfRecord := &CloudflareDNSRecord{
		ZoneID:   zoneID,
		RecordID: record.ID,
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Proxied:  *record.Proxied,
		Comment:  record.Comment,
	}
	
	if record.Priority != nil {
		cfRecord.Priority = int(*record.Priority)
	}
	
	return cfRecord, nil
}

// CreateDNSRecord creates a new DNS record
func (m *CloudflareManager) CreateDNSRecord(zoneID string, params DNSRecordParams) (*CloudflareDNSRecord, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}
	
	client, err := m.GetClient(zone.AccountID)
	if err != nil {
		return nil, err
	}
	
	ctx := context.Background()
	zoneIdentifier := cf.ZoneIdentifier(zoneID)
	
	createParams := cf.CreateDNSRecordParams{
		Type:    params.Type,
		Name:    params.Name,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: params.Comment,
	}
	
	if params.Priority != nil {
		priority := uint16(*params.Priority)
		createParams.Priority = &priority
	}
	
	if params.Proxied != nil {
		createParams.Proxied = params.Proxied
	}
	
	record, err := client.CreateDNSRecord(ctx, zoneIdentifier, createParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS record: %w", err)
	}
	
	cfRecord := &CloudflareDNSRecord{
		ZoneID:   zoneID,
		RecordID: record.ID,
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Proxied:  *record.Proxied,
		Comment:  params.Comment,
	}
	
	if record.Priority != nil {
		cfRecord.Priority = int(*record.Priority)
	}
	
	// Save to database
	memory.Create[*CloudflareDNSRecord](m.db, "cloudflare_dns_records", cfRecord)
	
	return cfRecord, nil
}

// UpdateDNSRecord updates an existing DNS record
func (m *CloudflareManager) UpdateDNSRecord(zoneID, recordID string, params DNSRecordParams) (*CloudflareDNSRecord, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}
	
	client, err := m.GetClient(zone.AccountID)
	if err != nil {
		return nil, err
	}
	
	ctx := context.Background()
	zoneIdentifier := cf.ZoneIdentifier(zoneID)
	
	updateParams := cf.UpdateDNSRecordParams{
		ID:      recordID,
		Type:    params.Type,
		Name:    params.Name,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: &params.Comment,
	}
	
	if params.Priority != nil {
		priority := uint16(*params.Priority)
		updateParams.Priority = &priority
	}
	
	if params.Proxied != nil {
		updateParams.Proxied = params.Proxied
	}
	
	record, err := client.UpdateDNSRecord(ctx, zoneIdentifier, updateParams)
	if err != nil {
		return nil, fmt.Errorf("failed to update DNS record: %w", err)
	}
	
	cfRecord := &CloudflareDNSRecord{
		ZoneID:   zoneID,
		RecordID: record.ID,
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Proxied:  *record.Proxied,
		Comment:  params.Comment,
	}
	
	if record.Priority != nil {
		cfRecord.Priority = int(*record.Priority)
	}
	
	return cfRecord, nil
}

// DeleteDNSRecord deletes a DNS record
func (m *CloudflareManager) DeleteDNSRecord(zoneID, recordID string) error {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return err
	}
	
	client, err := m.GetClient(zone.AccountID)
	if err != nil {
		return err
	}
	
	ctx := context.Background()
	zoneIdentifier := cf.ZoneIdentifier(zoneID)
	
	if err := client.DeleteDNSRecord(ctx, zoneIdentifier, recordID); err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}
	
	// Remove from database
	records := memory.Filter[*CloudflareDNSRecord](m.db, "cloudflare_dns_records", func(r *CloudflareDNSRecord) bool {
		return r.RecordID == recordID
	})
	
	if len(records) > 0 {
		memory.Delete[*CloudflareDNSRecord](m.db, "cloudflare_dns_records", records[0].ID)
	}
	
	return nil
}

// FindDNSRecord finds an existing DNS record by type and name
func (m *CloudflareManager) FindDNSRecord(zoneID, recordType, recordName string) (*CloudflareDNSRecord, error) {
	records, err := m.ListDNSRecords(zoneID)
	if err != nil {
		return nil, err
	}
	
	for _, record := range records {
		if record.Type == recordType && record.Name == recordName {
			return record, nil
		}
	}
	
	return nil, nil // Not found
}

// UpsertDNSRecord creates or updates a DNS record
func (m *CloudflareManager) UpsertDNSRecord(zoneID string, params DNSRecordParams) (*CloudflareDNSRecord, error) {
	// Try to find existing record with same type and name
	existingRecord, err := m.FindDNSRecord(zoneID, params.Type, params.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing record: %w", err)
	}
	
	if existingRecord != nil {
		// Update existing record
		return m.UpdateDNSRecord(zoneID, existingRecord.RecordID, params)
	}
	
	// Check for conflicting records with same name but different type (e.g., CNAME vs A)
	allRecords, err := m.ListDNSRecords(zoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to list DNS records: %w", err)
	}
	
	for _, record := range allRecords {
		if record.Name == params.Name && record.Type != params.Type {
			// Found conflicting record - delete it
			log.Printf("⚠️  Found conflicting %s record for %s, deleting before creating %s record", 
				record.Type, params.Name, params.Type)
			if err := m.DeleteDNSRecord(zoneID, record.RecordID); err != nil {
				log.Printf("❌ Failed to delete conflicting record: %v", err)
				return nil, fmt.Errorf("failed to delete conflicting %s record: %w", record.Type, err)
			}
		}
	}
	
	// Create new record
	return m.CreateDNSRecord(zoneID, params)
}

// CreateEmailDNSRecords creates all necessary DNS records for email server
func (m *CloudflareManager) CreateEmailDNSRecords(zoneID string, params EmailDNSParams) error {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return err
	}
	
	// Extract domain from zone
	domain := zone.Name
	
	// Generate DKIM key if not provided
	if params.DKIMRecord == "" {
		log.Printf("⚠️  DKIM record not provided, generating new key...")
		privateKey, publicKey, err := generateDKIMKeysForDNS()
		if err != nil {
			return fmt.Errorf("failed to generate DKIM keys: %w", err)
		}
		params.DKIMRecord = publicKey
		// Note: Private key is not stored, this is just for DNS setup
		_ = privateKey // Private key would need to be saved to email server config
	}
	
	// Set defaults if not provided
	if params.DKIMSelector == "" {
		params.DKIMSelector = "mail"
	}
	
	// Build SPF record with server IP if provided
	if params.SPFRecord == "" {
		if params.MailServerIP != "" {
			// Include server IP in SPF for production
			params.SPFRecord = fmt.Sprintf("v=spf1 a mx ip4:%s -all", params.MailServerIP)
			log.Printf("🔐 SPF record includes server IP: %s", params.MailServerIP)
		} else {
			// Fallback without IP
			params.SPFRecord = "v=spf1 a mx ~all"
			log.Printf("⚠️  SPF record created without server IP (not recommended for production)")
		}
	}
	
	// Strict DMARC policy for production
	if params.DMARCRecord == "" {
		params.DMARCRecord = fmt.Sprintf("v=DMARC1; p=reject; rua=mailto:dmarc@%s; ruf=mailto:forensics@%s; fo=1; adkim=s; aspf=s", domain, domain)
		log.Printf("🔐 DMARC policy set to 'reject' for maximum protection")
	}
	
	if params.MXRecord == "" {
		params.MXRecord = "mail." + domain
	}
	
	// MX Record
	priority := 10
	if _, err := m.UpsertDNSRecord(zone.ZoneID, DNSRecordParams{
		Type:     "MX",
		Name:     domain,
		Content:  params.MXRecord,
		TTL:      1,
		Priority: &priority,
		Comment:  "Email server MX record",
	}); err != nil {
		return fmt.Errorf("failed to upsert MX record: %w", err)
	}
	
	// SPF Record
	spfContent := strings.TrimSpace(params.SPFRecord)
	if _, err := m.UpsertDNSRecord(zone.ZoneID, DNSRecordParams{
		Type:    "TXT",
		Name:    domain,
		Content: spfContent,
		TTL:     1,
		Comment: "SPF record for email authentication",
	}); err != nil {
		log.Printf("❌ SPF record error: %v", err)
		return fmt.Errorf("failed to upsert SPF record: %w", err)
	}
	
	// DKIM Record
	// Cloudflare sometimes needs DKIM record without quotes for very long values
	dkimContent := params.DKIMRecord
	// Ensure no extra whitespace or newlines
	dkimContent = strings.TrimSpace(dkimContent)
	dkimContent = strings.ReplaceAll(dkimContent, "\n", "")
	dkimContent = strings.ReplaceAll(dkimContent, "\r", "")

	if _, err := m.UpsertDNSRecord(zone.ZoneID, DNSRecordParams{
		Type:    "TXT",
		Name:    params.DKIMSelector + "._domainkey." + domain,
		Content: dkimContent,
		TTL:     1,
		Comment: "DKIM public key for email signing",
	}); err != nil {
		log.Printf("❌ DKIM record error - Full content: %s", dkimContent)
		return fmt.Errorf("failed to upsert DKIM record: %w", err)
	}
	
	// DMARC Record
	dmarcContent := strings.TrimSpace(params.DMARCRecord)
	if _, err := m.UpsertDNSRecord(zone.ZoneID, DNSRecordParams{
		Type:    "TXT",
		Name:    "_dmarc." + domain,
		Content: dmarcContent,
		TTL:     1,
		Comment: "DMARC policy for email authentication",
	}); err != nil {
		log.Printf("❌ DMARC record error: %v", err)
		return fmt.Errorf("failed to upsert DMARC record: %w", err)
	}
	
	// A Record for mail server (if provided)
	if params.MailServerIP != "" {
		proxied := false
		if _, err := m.UpsertDNSRecord(zone.ZoneID, DNSRecordParams{
			Type:    "A",
			Name:    "mail." + domain,
			Content: params.MailServerIP,
			TTL:     1,
			Proxied: &proxied,
			Comment: "Mail server A record",
		}); err != nil {
			return fmt.Errorf("failed to upsert mail server A record: %w", err)
		}
	}
	
	return nil
}

// UpdateEmailDNSRecords updates email DNS records (uses UpsertDNSRecord internally)
func (m *CloudflareManager) UpdateEmailDNSRecords(zoneID string, params EmailDNSParams) error {
	// CreateEmailDNSRecords already uses UpsertDNSRecord which updates existing records
	return m.CreateEmailDNSRecords(zoneID, params)
}

// EmailDNSParams represents parameters for email DNS records
type EmailDNSParams struct {
	MXRecord      string
	SPFRecord     string
	DKIMRecord    string
	DKIMSelector  string
	DMARCRecord   string
	MailServerIP  string
}

// generateDKIMKeysForDNS generates RSA key pair for DKIM (DNS setup only)
func generateDKIMKeysForDNS() (privateKeyPEM, publicKeyTXT string, err error) {
	// Generate 2048-bit RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	
	// Private key to PEM
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}))
	
	// Public key to DNS TXT format
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	
	// Convert to DKIM DNS format (base64, no headers, no whitespace)
	publicKeyStr := string(publicKeyPEM)
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "-----BEGIN PUBLIC KEY-----", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "-----END PUBLIC KEY-----", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "\n", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "\r", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, " ", "")
	publicKeyStr = strings.TrimSpace(publicKeyStr)
	
	// Format for DNS TXT record
	publicKeyTXT = fmt.Sprintf("v=DKIM1; k=rsa; p=%s", publicKeyStr)
	
	return privateKeyPEM, publicKeyTXT, nil
}
