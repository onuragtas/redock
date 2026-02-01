package controllers

import (
	"redock/tunnel_proxy"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	tunnel_models "github.com/onuragtas/tunnel-client/models"
)

const tunnelTokenHeader = "X-Tunnel-Token"

func requireTunnelToken(c *fiber.Ctx) (string, bool) {
	token := c.Get(tunnelTokenHeader)
	if token == "" {
		return "", false
	}
	return token, true
}

// UpdateDockerImages method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func CheckUser(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}
	check := tunnel_proxy.GetTunnelProxy().CheckUser(token)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"login": check,
		},
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelLogin(c *fiber.Ctx) error {

	type Login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	model := &Login{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	check := tunnel_proxy.GetTunnelProxy().Login(model.Username, model.Password)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": !check.Success,
		"msg":   nil,
		"data":  check.Data,
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelRegister(c *fiber.Ctx) error {

	type Model struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	model := &Model{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	check := tunnel_proxy.GetTunnelProxy().Register(model.Username, model.Password, model.Email)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": check.Success,
		"msg":   nil,
		"data":  check.Data,
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelLogout(c *fiber.Ctx) error {
	tunnel_proxy.GetTunnelProxy().Logout()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// TunnelList method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelList(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	type DomainItem struct {
		CreatedAt time.Time   `json:"CreatedAt"`
		UpdatedAt time.Time   `json:"UpdatedAt"`
		DeletedAt interface{} `json:"DeletedAt"`
		ID        int         `json:"id"`
		UserID    int         `json:"user_id"`
		Domain    string      `json:"domain"`
		Port      int         `json:"port"`
		KeepAlive int         `json:"keep_alive"`
		Started   bool        `json:"started"`
	}

	var list []DomainItem

	domains := tunnel_proxy.GetTunnelProxy().ListDomain(token)
	startedTunnels := tunnel_proxy.GetTunnelProxy().GetStartedList()

	for _, v := range domains.Data.Domains {
		item := DomainItem{
			CreatedAt: v.CreatedAt,
			UpdatedAt: v.UpdatedAt,
			DeletedAt: v.DeletedAt,
			ID:        v.ID,
			UserID:    v.UserID,
			Domain:    v.Domain,
			Port:      v.Port,
			KeepAlive: v.KeepAlive,
		}

		for _, started := range startedTunnels.Data {
			if v.Domain == started.Domain.Domain {
				item.Started = true
			}
		}

		list = append(list, item)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// TunnelDelete method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelDelete(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	model := &tunnel_models.DomainItem{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	list := tunnel_proxy.GetTunnelProxy().DeleteDomain(strconv.Itoa(model.ID), token)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// TunnelAdd method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelAdd(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	model := &tunnel_models.DomainItem{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	list := tunnel_proxy.GetTunnelProxy().AddDomain(model.Domain, token)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// TunnelStart method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelStart(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	model := &tunnel_models.Tunnel{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	list := []tunnel_models.Tunnel{*model}

	tunnel_proxy.GetTunnelProxy().ListDomain(token)
	tunnel_proxy.GetTunnelProxy().StartTunnel(list)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// TunnelStop method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelStop(c *fiber.Ctx) error {
	if _, ok := requireTunnelToken(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	model := &tunnel_models.Tunnel{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	tunnel_proxy.GetTunnelProxy().StopTunnel(model.Domain)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelUserInfo(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	check := tunnel_proxy.GetTunnelProxy().UserInfo(token)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": !check.Success,
		"msg":   nil,
		"data":  check.Data.User,
	})
}

func TunnelRenewDomain(c *fiber.Ctx) error {
	token, ok := requireTunnelToken(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "X-Tunnel-Token required",
		})
	}

	model := &tunnel_models.DomainItem{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	tunnel_proxy.GetTunnelProxy().RenewDomain(model.Domain, token)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
	})
}
