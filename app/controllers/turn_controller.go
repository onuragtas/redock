package controllers

import (
	"redock/turn_server"

	"github.com/gofiber/fiber/v2"
)

func turnManager(c *fiber.Ctx) (*turn_server.Manager, error) {
	m := turn_server.GetManager()
	if m == nil {
		return nil, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "TURN server not initialized",
		})
	}
	return m, nil
}

func turnResult(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "msg": nil, "data": data})
}

// turnHandler wraps the boilerplate: resolve the manager, run fn.
func turnHandler(fn func(c *fiber.Ctx, m *turn_server.Manager) (any, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		m, err := turnManager(c)
		if m == nil {
			return err
		}
		data, ferr := fn(c, m)
		return turnResult(c, data, ferr)
	}
}

type turnNameBody struct {
	Name string `json:"name"`
}

var (
	TURNStatus = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.Status(), nil
	})
	TURNGetConfig = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.Config()
	})
	TURNUpdateConfig = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in turn_server.TURNConfigEntity
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		if err := m.UpdateConfig(in); err != nil {
			return nil, err
		}
		return m.Status(), nil
	})
	TURNStart = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		err := m.Start()
		return m.Status(), err
	})
	TURNStop = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		err := m.Stop()
		return m.Status(), err
	})

	TURNListApps = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.ListApps(), nil
	})
	TURNCreateApp = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in turn_server.AppInput
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return m.CreateApp(in)
	})
	TURNUpdateApp = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in turn_server.AppInput
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return m.UpdateApp(in)
	})
	TURNRotateSecret = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in turnNameBody
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return m.RotateSecret(in.Name)
	})
	TURNDeleteApp = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in turnNameBody
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return nil, m.DeleteApp(in.Name)
	})
	TURNIssueCredentials = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in struct {
			App  string `json:"app"`
			User string `json:"user"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		creds, err := m.IssueCredentials(in.App, in.User)
		if err != nil {
			return nil, err
		}
		return fiber.Map{"credentials": creds, "urls": m.ICEServerURLs()}, nil
	})

	TURNSessions = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.Sessions(), nil
	})
	TURNKillSession = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in struct {
			ID      string `json:"id"`
			BlockIP bool   `json:"block_ip"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return nil, m.KillSession(in.ID, in.BlockIP)
	})
	TURNEvents = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.Events(), nil
	})
	TURNBlocked = turnHandler(func(_ *fiber.Ctx, m *turn_server.Manager) (any, error) {
		return m.Blocked(), nil
	})
	TURNUnblock = turnHandler(func(c *fiber.Ctx, m *turn_server.Manager) (any, error) {
		var in struct {
			IP string `json:"ip"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		m.Unblock(in.IP)
		return nil, nil
	})
)
