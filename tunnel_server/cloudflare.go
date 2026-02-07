package tunnel_server

import (
	"fmt"
	"redock/cloudflare"
)

// UpdateTunnelDNSRecord updates an existing A record's content (e.g. server IP) by zone ID and record ID.
func UpdateTunnelDNSRecord(zoneID, recordID, name, serverIP string) error {
	if zoneID == "" || recordID == "" || name == "" || serverIP == "" {
		return fmt.Errorf("zoneID, recordID, name and serverIP required")
	}
	mgr := cloudflare.GetCloudflareManager()
	if mgr == nil {
		return fmt.Errorf("cloudflare manager not initialized")
	}
	proxied := false
	_, err := mgr.UpdateDNSRecord(zoneID, recordID, cloudflare.DNSRecordParams{
		Type:    "A",
		Name:   name,
		Content: serverIP,
		TTL:    1,
		Proxied: &proxied,
		Comment: "tunnel",
	})
	return err
}

// CreateTunnelDNSRecord creates an A record for a tunnel subdomain in the configured zone.
// name: full DNS name (e.g. subdomain.suffix or "myapp.tnpx.org")
// serverIP: value for the A record (public IP of tunnel server)
// Returns Cloudflare record ID for later delete.
func CreateTunnelDNSRecord(zoneID, name, serverIP string) (recordID string, err error) {
	if zoneID == "" || name == "" || serverIP == "" {
		return "", fmt.Errorf("zoneID, name and serverIP required")
	}

	mgr := cloudflare.GetCloudflareManager()
	if mgr == nil {
		return "", fmt.Errorf("cloudflare manager not initialized")
	}

	proxied := false
	record, err := mgr.CreateDNSRecord(zoneID, cloudflare.DNSRecordParams{
		Type:    "A",
		Name:   name,
		Content: serverIP,
		TTL:    1,
		Proxied: &proxied,
		Comment: "tunnel",
	})
	if err != nil {
		return "", fmt.Errorf("create tunnel A record: %w", err)
	}

	return record.RecordID, nil
}

// DeleteTunnelDNSRecord deletes the A record by zone ID and Cloudflare record ID.
func DeleteTunnelDNSRecord(zoneID, recordID string) error {
	if zoneID == "" || recordID == "" {
		return fmt.Errorf("zoneID and recordID required")
	}

	mgr := cloudflare.GetCloudflareManager()
	if mgr == nil {
		return fmt.Errorf("cloudflare manager not initialized")
	}

	return mgr.DeleteDNSRecord(zoneID, recordID)
}
