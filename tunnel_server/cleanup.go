package tunnel_server

import (
	"log"
	"sort"
	"time"
)

const cleanupInterval = 24 * time.Hour

// CleanupUnusedDomains deletes domains not used within UnusedDomainTTLDays, then compacts port assignments.
// Safe to call from a goroutine. No-op if UnusedDomainTTLDays <= 0.
func CleanupUnusedDomains() {
	cfg := GetConfig()
	if cfg == nil || cfg.UnusedDomainTTLDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -cfg.UnusedDomainTTLDays)
	all := AllDomains()
	var toDelete []*TunnelDomain
	for _, d := range all {
		var lastUsed time.Time
		if d.LastUsedAt != nil {
			lastUsed = *d.LastUsedAt
		} else {
			lastUsed = d.CreatedAt
		}
		if lastUsed.Before(cutoff) {
			toDelete = append(toDelete, d)
		}
	}
	for _, d := range toDelete {
		if cfg.CloudflareZoneID != "" && d.CloudflareRecordID != "" {
			_ = DeleteTunnelDNSRecord(cfg.CloudflareZoneID, d.CloudflareRecordID)
		}
		_ = RemoveTunnelDomainFromGateway(d)
		if err := DeleteDomainByID(d.ID); err != nil {
			log.Printf("tunnel_server: cleanup delete domain %s: %v", d.FullDomain, err)
		} else {
			log.Printf("tunnel_server: cleanup deleted unused domain %s (last used before %s)", d.FullDomain, cutoff.Format(time.RFC3339))
		}
	}
	if len(toDelete) == 0 {
		return
	}
	compactPorts()
}

// compactPorts reassigns ports so inactive domains get contiguous ports from PortRangeStart.
// Domains that currently have an active tunnel (client bound) are left unchanged to avoid breaking live traffic.
func compactPorts() {
	cfg := GetConfig()
	if cfg == nil {
		return
	}
	all := AllDomains()
	if len(all) == 0 {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	nextPort := cfg.PortRangeStart
	for _, d := range all {
		// Do not change port for domains that currently have a tunnel running
		if GetClientByDomain(d.FullDomain) != nil {
			continue
		}
		newPort := nextPort
		nextPort++
		if d.Port == newPort {
			continue
		}
		_ = RemoveTunnelDomainFromGateway(d)
		d.Port = newPort
		d.GatewayServiceID = ""
		d.GatewayRouteID = ""
		d.GatewayUDPServiceID = ""
		d.GatewayUDPRouteID = ""
		d.GatewayTCPServiceID = ""
		d.GatewayTCPRouteID = ""
		if err := AddTunnelDomainToGateway(d); err != nil {
			log.Printf("tunnel_server: cleanup compact port for %s: %v", d.FullDomain, err)
			continue
		}
		if err := UpdateDomain(d); err != nil {
			log.Printf("tunnel_server: cleanup update domain %s: %v", d.FullDomain, err)
		} else {
			log.Printf("tunnel_server: cleanup compacted %s -> port %d", d.FullDomain, newPort)
		}
	}
}

// RunCleanupLoop runs CleanupUnusedDomains once after 1 minute, then every cleanupInterval. Call from a goroutine.
func RunCleanupLoop() {
	time.Sleep(1 * time.Minute)
	CleanupUnusedDomains()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		CleanupUnusedDomains()
	}
}
