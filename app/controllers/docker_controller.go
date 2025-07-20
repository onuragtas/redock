package controllers

import (
	"encoding/json"
	"log"
	"os"
	"redock/app/models"
	"redock/devenv"
	docker_manager "redock/docker-manager"
	"redock/selfupdate"
	"runtime"
	"strings"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/kardianos/osext"
	"github.com/onuragtas/command"
)

// GetEnv method to create a new user.
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
func GetEnv(c *fiber.Ctx) error {
	env := docker_manager.GetDockerManager().Env

	if env == "" {
		env = docker_manager.GetDockerManager().EnvDist
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"env": env,
		},
	})
}

// SetEnv method to create a new user.
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
func SetEnv(c *fiber.Ctx) error {
	envModel := &models.Env{}
	// Checking received data from JSON body.
	if err := c.BodyParser(envModel); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	manager := docker_manager.GetDockerManager()
	env := envModel.Env
	workdir := manager.GetWorkDir()
	err := os.WriteFile(workdir+"/.env", []byte(envModel.Env), 0777)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{})
	}

	manager.Env = env

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"env": env,
		},
	})
}

// Regenerate method to create a new user.
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
func Regenerate(c *fiber.Ctx) error {
	manager := docker_manager.GetDockerManager()
	manager.RegenerateXDebugConf()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// GetLocalIp method to create a new user.
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
func GetLocalIp(c *fiber.Ctx) error {
	manager := docker_manager.GetDockerManager()
	localIp := manager.GetLocalIP()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"ip": localIp,
		},
	})
}

// GetAllServices method to create a new user.
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
func GetAllServices(c *fiber.Ctx) error {
	type Service struct {
		ContainerName string   `json:"container_name"`
		Links         []string `json:"links"`
		DependsOn     []string `json:"depends_on"`
		Image         string   `json:"image"`
		Active        bool     `json:"active"`
	}

	var services []Service

	manager := docker_manager.GetDockerManager()

	for _, service := range manager.Services {
		serv := Service{
			ContainerName: service.ContainerName.(string),
			Links:         service.Links,
			DependsOn:     service.DependsOn,
			Image:         service.Image,
		}

		for _, activeService := range manager.ActiveServices {
			if activeService == service.ContainerName {
				serv.Active = true
			}
		}

		services = append(services, serv)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"all_services": services,
		},
	})
}

// GetAllVHosts method to create a new user.
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
func GetAllVHosts(c *fiber.Ctx) error {

	manager := docker_manager.GetDockerManager()
	list := manager.Virtualhost.VirtualHosts()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// GetPhpServices method to create a new user.
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
func GetPhpServices(c *fiber.Ctx) error {

	manager := docker_manager.GetDockerManager()
	list := manager.ActiveServices
	var services []string
	for _, service := range list {
		if strings.Contains(service, "php") {
			services = append(services, service)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  services,
	})
}

// GetDevEnv method to create a new user.
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
func GetDevEnv(c *fiber.Ctx) error {

	manager := docker_manager.GetDockerManager()
	file, err := os.ReadFile(manager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
	}

	var devEnvList []docker_manager.DevEnv
	json.Unmarshal(file, &devEnvList)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": err != nil,
		"msg":   err,
		"data":  devEnvList,
	})
}

// GetVHostContent method to create a new user.
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
func GetVHostContent(c *fiber.Ctx) error {

	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	file, err := os.ReadFile(model.Path)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": err != nil,
		"msg":   nil,
		"data":  string(file),
	})
}

// SetVHostContent method to create a new user.
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
func SetVHostContent(c *fiber.Ctx) error {

	type Parameter struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	err := os.WriteFile(model.Path, []byte(model.Content), 0777)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": err != nil,
		"msg":   nil,
		"data": fiber.Map{
			"content": model.Content,
		},
	})
}

// DeleteVHost method to create a new user.
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
func DeleteVHost(c *fiber.Ctx) error {

	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	err := os.Remove(model.Path)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": err != nil,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// CreateVHost method to create a new user.
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
func CreateVHost(c *fiber.Ctx) error {

	type Parameter struct {
		Domain            string `json:"domain"`
		Service           string `json:"service"`
		ConfigurationType string `json:"configurationType"`
		ProxyPass         string `json:"proxyPass"`
		Folder            string `json:"folder"`
		PhpService        string `json:"phpService"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if model.ConfigurationType == "Default" {
		model.ProxyPass = ""
	} else {
		model.Folder = ""
		model.PhpService = ""
	}

	manager := docker_manager.GetDockerManager()

	manager.AddVirtualHost(model.Service, model.Domain, model.Folder, model.PhpService, model.ConfigurationType, model.ProxyPass, true)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// CreateDevEnv method to create a new user.
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
func CreateDevEnv(c *fiber.Ctx) error {

	model := &devenv.DevEnvModel{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	result := devenv.GetDevEnvManager().AddDevEnv(model)

	if !result {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "User already exists or port/redockPort is already in use",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// EditDevEnv method to create a new user.
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
func EditDevEnv(c *fiber.Ctx) error {

	model := &devenv.DevEnvModel{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	result := devenv.GetDevEnvManager().EditDevEnv(model)

	if !result {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "User not found or port/redockPort is already in use",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// DeleteDevEnv method to create a new user.
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
func DeleteDevEnv(c *fiber.Ctx) error {

	model := &devenv.DevEnvModel{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	devenv.GetDevEnvManager().DeleteDevEnv(model.Username)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RegenerateDevEnv method to create a new user.
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
func RegenerateDevEnv(c *fiber.Ctx) error {
	devenv.GetDevEnvManager().Regenerate()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RegenerateDevEnv method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/install [get]
func Install(c *fiber.Ctx) error {
	devenv.GetDevEnvManager().Install()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// AddXDebug method to create a new user.
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
func AddXDebug(c *fiber.Ctx) error {
	docker_manager.GetDockerManager().AddXDebug()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RemoveXDebug method to create a new user.
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
func RemoveXDebug(c *fiber.Ctx) error {
	docker_manager.GetDockerManager().RemoveXDebug()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RestartNginxHttpd method to create a new user.
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
func RestartNginxHttpd(c *fiber.Ctx) error {
	docker_manager.GetDockerManager().RestartAll()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// SelfUpdate method to create a new user.
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
func SelfUpdate(c *fiber.Ctx) error {

	log.Println("https://github.com/onuragtas/redock/releases/latest/download/redock_"+runtime.GOOS+"_"+runtime.GOARCH, "downloading...")

	var updater = &selfupdate.Updater{
		CurrentVersion: "v1.0.0",
		BinURL:         "https://github.com/onuragtas/redock/releases/latest/download/redock_" + runtime.GOOS + "_" + runtime.GOARCH,
		Dir:            "update/",
		CmdName:        "/docker-env",
	}

	if updater != nil {
		log.Println("update: started, please wait...")
		err := updater.Update()
		if err != nil {
			log.Println("update error:", err)
		}
		path, _ := osext.Executable()
		log.Println("update: finished")
		if err := syscall.Exec(path, os.Args, os.Environ()); err != nil {
			panic(err)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// UpdateDocker method to create a new user.
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
func UpdateDocker(c *fiber.Ctx) error {

	docker_manager.GetDockerManager().UpdateDocker()
	docker_manager.GetDockerManager().Init()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
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
func UpdateDockerImages(c *fiber.Ctx) error {

	command := command.Command{}
	for _, service := range docker_manager.GetDockerManager().ActiveServicesList {
		if service.Image != "" {
			log.Println("docker pull", service.Image)
			command.RunWithPipe("docker", "pull", service.Image)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// AddService method to create a new user.
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
func AddService(c *fiber.Ctx) error {

	type Parameter struct {
		Service string `json:"service"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	docker_manager.GetDockerManager().AddService(model.Service)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RemoveService method to create a new user.
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
func RemoveService(c *fiber.Ctx) error {

	type Parameter struct {
		Service string `json:"service"`
	}

	model := &Parameter{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	docker_manager.GetDockerManager().RemoveService(model.Service)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}
