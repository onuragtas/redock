package controllers

import (
	"redock/api_gateway"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// APIGatewayGetConfig returns the current API Gateway configuration
// @Description Get the current API Gateway configuration
// @Summary get API gateway config
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {object} api_gateway.GatewayConfig
// @Router /v1/api_gateway/config [get]
func APIGatewayGetConfig(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  gw.GetConfig(),
	})
}

// APIGatewayUpdateConfig updates the API Gateway configuration
// @Description Update the API Gateway configuration
// @Summary update API gateway config
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param config body api_gateway.GatewayConfig true "Gateway configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/config [post]
func APIGatewayUpdateConfig(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	config := &api_gateway.GatewayConfig{}
	if err := c.BodyParser(config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := gw.UpdateConfig(config); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Configuration updated successfully",
		"data":  gw.GetConfig(),
	})
}

// APIGatewayStart starts the API Gateway
// @Description Start the API Gateway servers
// @Summary start API gateway
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/start [post]
func APIGatewayStart(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	if err := gw.Start(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "API Gateway started successfully",
	})
}

// APIGatewayStop stops the API Gateway
// @Description Stop the API Gateway servers
// @Summary stop API gateway
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/stop [post]
func APIGatewayStop(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	if err := gw.Stop(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "API Gateway stopped successfully",
	})
}

// APIGatewayStatus returns the current status of the API Gateway
// @Description Get the current status of the API Gateway
// @Summary get API gateway status
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/status [get]
func APIGatewayStatus(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	config := gw.GetConfig()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"running":       gw.IsRunning(),
			"http_port":     config.HTTPPort,
			"https_port":    config.HTTPSPort,
			"https_enabled": config.HTTPSEnabled,
			"enabled":       config.Enabled,
		},
	})
}

// APIGatewayGetStats returns the API Gateway statistics
// @Description Get the API Gateway statistics
// @Summary get API gateway stats
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {object} api_gateway.GatewayStats
// @Router /v1/api_gateway/stats [get]
func APIGatewayGetStats(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  gw.GetStats(),
	})
}

// APIGatewayGetServiceHealth returns the health status of all services
// @Description Get the health status of all upstream services
// @Summary get service health
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {array} api_gateway.ServiceHealth
// @Router /v1/api_gateway/health [get]
func APIGatewayGetServiceHealth(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  gw.GetServiceHealth(),
	})
}

// APIGatewayListServices returns all configured services
// @Description List all configured upstream services
// @Summary list services
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {array} api_gateway.Service
// @Router /v1/api_gateway/services [get]
func APIGatewayListServices(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	config := gw.GetConfig()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  config.Services,
	})
}

// APIGatewayAddService adds a new service
// @Description Add a new upstream service
// @Summary add service
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param service body api_gateway.Service true "Service configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/services [post]
func APIGatewayAddService(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	service := &api_gateway.Service{}
	if err := c.BodyParser(service); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Generate ID if not provided
	if service.ID == "" {
		service.ID = uuid.New().String()
	}

	if err := gw.AddService(*service); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Service added successfully",
		"data":  service,
	})
}

// APIGatewayUpdateService updates an existing service
// @Description Update an existing upstream service
// @Summary update service
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param service body api_gateway.Service true "Service configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/services [put]
func APIGatewayUpdateService(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	service := &api_gateway.Service{}
	if err := c.BodyParser(service); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if service.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Service ID is required",
		})
	}

	if err := gw.UpdateService(*service); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Service updated successfully",
		"data":  service,
	})
}

// APIGatewayDeleteService deletes a service
// @Description Delete an upstream service
// @Summary delete service
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param id path string true "Service ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/services/{id} [delete]
func APIGatewayDeleteService(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	type DeleteRequest struct {
		ID string `json:"id"`
	}

	req := &DeleteRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if req.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Service ID is required",
		})
	}

	if err := gw.DeleteService(req.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Service deleted successfully",
	})
}

// APIGatewayListRoutes returns all configured routes
// @Description List all configured routes
// @Summary list routes
// @Tags API Gateway
// @Accept json
// @Produce json
// @Success 200 {array} api_gateway.Route
// @Router /v1/api_gateway/routes [get]
func APIGatewayListRoutes(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	config := gw.GetConfig()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  config.Routes,
	})
}

// APIGatewayAddRoute adds a new route
// @Description Add a new route
// @Summary add route
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param route body api_gateway.Route true "Route configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/routes [post]
func APIGatewayAddRoute(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	route := &api_gateway.Route{}
	if err := c.BodyParser(route); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Generate ID if not provided
	if route.ID == "" {
		route.ID = uuid.New().String()
	}

	if err := gw.AddRoute(*route); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Route added successfully",
		"data":  route,
	})
}

// APIGatewayUpdateRoute updates an existing route
// @Description Update an existing route
// @Summary update route
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param route body api_gateway.Route true "Route configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/routes [put]
func APIGatewayUpdateRoute(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	route := &api_gateway.Route{}
	if err := c.BodyParser(route); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if route.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Route ID is required",
		})
	}

	if err := gw.UpdateRoute(*route); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Route updated successfully",
		"data":  route,
	})
}

// APIGatewayDeleteRoute deletes a route
// @Description Delete a route
// @Summary delete route
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param id path string true "Route ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/routes/{id} [delete]
func APIGatewayDeleteRoute(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	type DeleteRequest struct {
		ID string `json:"id"`
	}

	req := &DeleteRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if req.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Route ID is required",
		})
	}

	if err := gw.DeleteRoute(req.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Route deleted successfully",
	})
}

// APIGatewayTestUpstream tests connectivity to an upstream service
// @Description Test connectivity to an upstream service
// @Summary test upstream
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param request body object true "Test request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/test_upstream [post]
func APIGatewayTestUpstream(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	type TestRequest struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		Path string `json:"path"`
	}

	req := &TestRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if req.Host == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Host is required",
		})
	}

	if req.Port == 0 {
		req.Port = 80
	}

	if req.Path == "" {
		req.Path = "/"
	}

	statusCode, latency, err := gw.TestUpstream(req.Host, req.Port, req.Path)
	if err != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data": fiber.Map{
				"reachable":  false,
				"latency_ms": latency,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"reachable":   true,
			"status_code": statusCode,
			"latency_ms":  latency,
		},
	})
}

// APIGatewayHealthCheckNow triggers an immediate health check for a service
// @Description Trigger an immediate health check for a service
// @Summary health check now
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param id path string true "Service ID"
// @Success 200 {object} api_gateway.ServiceHealth
// @Router /v1/api_gateway/health_check/{id} [post]
func APIGatewayHealthCheckNow(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	type HealthCheckRequest struct {
		ServiceID string `json:"service_id"`
	}

	req := &HealthCheckRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if req.ServiceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Service ID is required",
		})
	}

	health, err := gw.HealthCheckNow(req.ServiceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  health,
	})
}

// APIGatewayValidateRoute validates a request against configured routes
// @Description Validate a request against configured routes
// @Summary validate route
// @Tags API Gateway
// @Accept json
// @Produce json
// @Param request body object true "Validation request"
// @Success 200 {object} map[string]interface{}
// @Router /v1/api_gateway/validate [post]
func APIGatewayValidateRoute(c *fiber.Ctx) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway not initialized",
		})
	}

	type ValidateRequest struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Host    string            `json:"host"`
		Headers map[string]string `json:"headers"`
	}

	req := &ValidateRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if req.Method == "" {
		req.Method = "GET"
	}

	if req.Path == "" {
		req.Path = "/"
	}

	if req.Host == "" {
		req.Host = "localhost"
	}

	route, service, err := gw.Validate(req.Method, req.Path, req.Host, req.Headers)
	if err != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data": fiber.Map{
				"matched": false,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"matched": true,
			"route":   route,
			"service": service,
		},
	})
}
