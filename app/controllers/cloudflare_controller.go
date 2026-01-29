package controllers

import (
	"fmt"
	"redock/cloudflare"
	"redock/platform/memory"
	
	"github.com/gofiber/fiber/v2"
)

// AddCloudflareAccount adds a new Cloudflare account
func AddCloudflareAccount(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	var req struct {
		Name     string `json:"name" validate:"required"`
		Email    string `json:"email" validate:"required,email"`
		APIKey   string `json:"api_key"`
		APIToken string `json:"api_token"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	account, err := manager.AddAccount(req.Name, req.Email, req.APIKey, req.APIToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to add account: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Cloudflare account added successfully",
		"data":  account,
	})
}

// GetCloudflareAccounts returns all Cloudflare accounts
func GetCloudflareAccounts(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	accounts := memory.FindAll[*cloudflare.CloudflareAccount](db, "cloudflare_accounts")
	
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  accounts,
	})
}

// DeleteCloudflareAccount deletes a Cloudflare account
func DeleteCloudflareAccount(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	accountIDParam := c.Params("account_id")
	var accountID uint
	if _, err := fmt.Sscanf(accountIDParam, "%d", &accountID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid account ID",
		})
	}

	if err := manager.RemoveAccount(accountID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete account: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Account deleted successfully",
	})
}

// SyncCloudflareZones syncs zones from Cloudflare
func SyncCloudflareZones(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	accountIDParam := c.Params("account_id")
	var accountID uint
	if _, err := fmt.Sscanf(accountIDParam, "%d", &accountID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid account ID",
		})
	}

	if err := manager.SyncZones(accountID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to sync zones: " + err.Error(),
		})
	}

	// Get zone count
	db, _ := manager.GetDB()
	zones := memory.Filter[*cloudflare.CloudflareZone](db, "cloudflare_zones", func(z *cloudflare.CloudflareZone) bool {
		return z.AccountID == accountID
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Zones synced successfully",
		"data": fiber.Map{
			"count": len(zones),
		},
	})
}

// GetCloudflareZones returns all zones
func GetCloudflareZones(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	accountIDParam := c.Query("account_id")
	if accountIDParam == "" {
		db, _ := manager.GetDB()
		zones := memory.FindAll[*cloudflare.CloudflareZone](db, "cloudflare_zones")
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"data":  zones,
		})
	}

	var accountID uint
	if _, err := fmt.Sscanf(accountIDParam, "%d", &accountID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid account ID",
		})
	}

	zones, err := manager.ListZones(accountID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get zones: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  zones,
	})
}

// GetCloudflareDNSRecords returns DNS records for a zone
func GetCloudflareDNSRecords(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	zoneIDParam := c.Params("zone_id")
	if zoneIDParam == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Zone ID is required",
		})
	}

	// Convert DB ID to Cloudflare Zone ID
	var zoneDBID uint
	if _, err := fmt.Sscanf(zoneIDParam, "%d", &zoneDBID); err != nil {
		// Maybe it's already a Cloudflare Zone ID (string), try direct
		records, err := manager.ListDNSRecords(zoneIDParam)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to get DNS records: " + err.Error(),
			})
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"data":  records,
		})
	}

	// Get zone from DB by our ID
	db, _ := manager.GetDB()
	zone, _ := memory.FindByID[*cloudflare.CloudflareZone](db, "cloudflare_zones", zoneDBID)
	if zone == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Zone not found",
		})
	}

	records, err := manager.ListDNSRecords(zone.ZoneID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get DNS records: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  records,
	})
}

// CreateCloudflareDNSRecord creates a new DNS record
func CreateCloudflareDNSRecord(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	zoneIDParam := c.Params("zone_id")
	if zoneIDParam == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Zone ID is required",
		})
	}

	var params cloudflare.DNSRecordParams
	if err := c.BodyParser(&params); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	// Convert DB ID to Cloudflare Zone ID if needed
	cloudflareZoneID := zoneIDParam
	var zoneDBID uint
	if _, err := fmt.Sscanf(zoneIDParam, "%d", &zoneDBID); err == nil {
		db, _ := manager.GetDB()
		zone, _ := memory.FindByID[*cloudflare.CloudflareZone](db, "cloudflare_zones", zoneDBID)
		if zone == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Zone not found",
			})
		}
		cloudflareZoneID = zone.ZoneID
	}

	record, err := manager.CreateDNSRecord(cloudflareZoneID, params)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to create DNS record: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS record created successfully",
		"data":  record,
	})
}

// UpdateCloudflareDNSRecord updates a DNS record
func UpdateCloudflareDNSRecord(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	zoneIDParam := c.Params("zone_id")
	recordID := c.Params("record_id")
	if zoneIDParam == "" || recordID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Zone ID and Record ID are required",
		})
	}

	var params cloudflare.DNSRecordParams
	if err := c.BodyParser(&params); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	// Convert DB ID to Cloudflare Zone ID if needed
	cloudflareZoneID := zoneIDParam
	var zoneDBID uint
	if _, err := fmt.Sscanf(zoneIDParam, "%d", &zoneDBID); err == nil {
		db, _ := manager.GetDB()
		zone, _ := memory.FindByID[*cloudflare.CloudflareZone](db, "cloudflare_zones", zoneDBID)
		if zone == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Zone not found",
			})
		}
		cloudflareZoneID = zone.ZoneID
	}

	record, err := manager.UpdateDNSRecord(cloudflareZoneID, recordID, params)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update DNS record: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS record updated successfully",
		"data":  record,
	})
}

// DeleteCloudflareDNSRecord deletes a DNS record
func DeleteCloudflareDNSRecord(c *fiber.Ctx) error {
	manager := cloudflare.GetCloudflareManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Cloudflare manager not initialized",
		})
	}

	zoneIDParam := c.Params("zone_id")
	recordID := c.Params("record_id")
	if zoneIDParam == "" || recordID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Zone ID and Record ID are required",
		})
	}

	// Convert DB ID to Cloudflare Zone ID if needed
	cloudflareZoneID := zoneIDParam
	var zoneDBID uint
	if _, err := fmt.Sscanf(zoneIDParam, "%d", &zoneDBID); err == nil {
		db, _ := manager.GetDB()
		zone, _ := memory.FindByID[*cloudflare.CloudflareZone](db, "cloudflare_zones", zoneDBID)
		if zone == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Zone not found",
			})
		}
		cloudflareZoneID = zone.ZoneID
	}

	if err := manager.DeleteDNSRecord(cloudflareZoneID, recordID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete DNS record: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS record deleted successfully",
	})
}

