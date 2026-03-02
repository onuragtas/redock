package controllers

import (
	"redock/deployment"

	"github.com/gofiber/fiber/v2"
)

// DeploymentList returns the list of deployment projects
func DeploymentList(c *fiber.Ctx) error {
	projects := deployment.GetDeployment().GetList()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  projects,
	})
}

// DeploymentAdd adds a new deployment project
func DeploymentAdd(c *fiber.Ctx) error {
	var project deployment.DeploymentProjectEntity
	if err := c.BodyParser(&project); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	err := deployment.GetDeployment().AddProject(&project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  &project,
	})
}

// DeploymentDelete deletes a deployment project by path
func DeploymentDelete(c *fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	err := deployment.GetDeployment().DeleteProject(req.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// DeploymentUpdate updates an existing deployment project
func DeploymentUpdate(c *fiber.Ctx) error {
	var project deployment.DeploymentProjectEntity
	if err := c.BodyParser(&project); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	err := deployment.GetDeployment().UpdateProject(&project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  &project,
	})
}

// DeploymentGet returns a deployment project by path
func DeploymentGet(c *fiber.Ctx) error {
	path := c.Query("path")
	project, err := deployment.GetDeployment().GetProjectByPath(path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  project,
	})
}

// DeploymentSetCredentials sets the username, token, and checkTime for deployment
func DeploymentSetCredentials(c *fiber.Ctx) error {
	var req struct {
		Username  string `json:"username"`
		Token     string `json:"token"`
		CheckTime *int   `json:"checkTime"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	err := deployment.GetDeployment().SetCredentials(req.Username, req.Token, req.CheckTime)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// DeploymentGetSettings returns the deployment config (username, token, checkTime)
func DeploymentGetSettings(c *fiber.Ctx) error {
	config := deployment.GetDeployment().GetConfig()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"username":  config.Username,
			"token":     config.Token,
			"checkTime": config.Settings.CheckTime,
		},
	})
}
