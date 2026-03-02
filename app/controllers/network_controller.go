package controllers

import (
	"strings"

	docker_manager "redock/docker-manager"
	"redock/pkg/network"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// NetworkListInterfaces returns the list of network interfaces (for IP alias UI).
func NetworkListInterfaces(c *fiber.Ctx) error {
	list, err := network.ListInterfaces()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  list,
	})
}

// NetworkListAddresses returns IPv4 addresses on the given interface.
func NetworkListAddresses(c *fiber.Ctx) error {
	iface := c.Query("interface")
	if iface == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "interface parameter is required",
			"data":  nil,
		})
	}
	addrs, err := network.ListAddresses(iface)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  addrs,
	})
}

// NetworkAddAliasRequest is the body for adding IP alias range.
type NetworkAddAliasRequest struct {
	Interface   string `json:"interface"`
	CIDROrRange string `json:"cidr_or_range"` // Örn. "88.255.136.0/24" veya "88.255.136.1-88.255.136.254"
}

// NetworkAddAlias adds IP addresses to an interface (netlink).
func NetworkAddAlias(c *fiber.Ctx) error {
	var req NetworkAddAliasRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request: " + err.Error(),
			"data":  nil,
		})
	}
	if req.Interface == "" || req.CIDROrRange == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "interface and cidr_or_range are required",
			"data":  nil,
		})
	}
	ipNets, err := network.ParseIPRange(req.CIDROrRange)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	added, err := network.AddAliases(req.Interface, ipNets)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	db := database.GetMemoryDB()
	if err := memory.Create[*network.PersistedIPAlias](db, network.TableIPAliases, &network.PersistedIPAlias{
		Interface:   req.Interface,
		CIDROrRange: strings.TrimSpace(req.CIDROrRange),
	}); err != nil {
		// Kernel'e eklendi ama DB'ye yazılamadı; reboot'ta bu blok tekrar uygulanmaz
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"msg":   nil,
			"data":  fiber.Map{"added": added, "total": len(ipNets), "persisted": false},
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{"added": added, "total": len(ipNets), "persisted": true},
	})
}

// NetworkRemoveAlias removes IP addresses from an interface.
func NetworkRemoveAlias(c *fiber.Ctx) error {
	var req NetworkAddAliasRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request: " + err.Error(),
			"data":  nil,
		})
	}
	if req.Interface == "" || req.CIDROrRange == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "interface and cidr_or_range are required",
			"data":  nil,
		})
	}
	ipNets, err := network.ParseIPRange(req.CIDROrRange)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	removed, err := network.RemoveAliases(req.Interface, ipNets)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	cidrTrimmed := strings.TrimSpace(req.CIDROrRange)
	all := memory.FindAll[*network.PersistedIPAlias](database.GetMemoryDB(), network.TableIPAliases)
	for _, a := range all {
		if a.Interface == req.Interface && a.CIDROrRange == cidrTrimmed {
			_ = memory.Delete[*network.PersistedIPAlias](database.GetMemoryDB(), network.TableIPAliases, a.GetID())
			break
		}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{"removed": removed, "total": len(ipNets)},
	})
}

// NetworkClientCommand returns the Redock server IP/host to use in the client's route command.
// "route add -net DEST GATEWAY" için GATEWAY sadece IP/host olmalı (port yok).
func NetworkClientCommand(c *fiber.Ctx) error {
	host := c.Get("Host")
	if host == "" {
		host = c.Hostname()
	}
	// Port varsa kaldır (route komutunda sadece IP/host kullanılır)
	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}
	host = strings.TrimSpace(host)
	if host == "localhost" || host == "" {
		if mgr := docker_manager.GetDockerManager(); mgr != nil {
			if localIP := mgr.GetLocalIP(); localIP != "" {
				host = localIP
			} else {
				host = "127.0.0.1"
			}
		} else {
			host = "127.0.0.1"
		}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  fiber.Map{"gateway_ip": host},
	})
}
