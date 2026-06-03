package controllers

import (
	"redock/onion_proxy"

	"github.com/gofiber/fiber/v2"
)

// OnionStatus reports whether `tor` is installed on the host, plus the
// current state of the manager. Frontend uses this to either enable the
// "Add Onion" flow or show platform-specific install instructions.
func OnionStatus(c *fiber.Ctx) error {
	m := onion_proxy.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "onion_proxy not initialized",
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false, "data": m.Status(),
	})
}

// OnionList returns all configured onion services.
func OnionList(c *fiber.Ctx) error {
	m := onion_proxy.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "onion_proxy not initialized",
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false, "data": m.List(),
	})
}

// OnionCreate provisions a new hidden service.
//
// Body: { "name": "...", "route_id": "...", "target_host": "...",
//         "target_port": 0, "virtual_port": 0 }
// route_id verilirse gateway entegrasyon modu; aksi halde target_host:port
// direkt forward edilir.
func OnionCreate(c *fiber.Ctx) error {
	m := onion_proxy.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "onion_proxy not initialized",
		})
	}
	in := onion_proxy.CreateInput{}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "msg": err.Error(),
		})
	}
	e, err := m.Create(in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "msg": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false, "data": e,
	})
}

// OnionUpdate edits an existing hidden service. Body fields are optional
// pointers: omitted fields stay unchanged.
//   { "name": "...", "route_id": "...", "enabled": true }
// The .onion address is always preserved (republish uses the same key).
func OnionUpdate(c *fiber.Ctx) error {
	m := onion_proxy.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "onion_proxy not initialized",
		})
	}
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "msg": "invalid id",
		})
	}
	in := onion_proxy.EditInput{}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "msg": err.Error(),
		})
	}
	e, err := m.Update(uint(id), in)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "msg": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false, "data": e,
	})
}

// OnionDelete removes a hidden service by ID (uint, route param).
func OnionDelete(c *fiber.Ctx) error {
	m := onion_proxy.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "onion_proxy not initialized",
		})
	}
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "msg": "invalid id",
		})
	}
	if err := m.Delete(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "msg": err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false})
}
