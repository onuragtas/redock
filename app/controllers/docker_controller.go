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

// GetServiceSettings exposes docker service customization inputs.
func GetServiceSettings(c *fiber.Ctx) error {
	manager := docker_manager.GetDockerManager()
	settings := manager.GetServiceSettings()
	metadata := manager.ListServiceMetadata()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"settings": settings,
			"services": metadata,
		},
	})
}

// UpdateServiceSettings lets contributors store container prefixes and port overrides.
func UpdateServiceSettings(c *fiber.Ctx) error {
	type Request struct {
		ContainerNamePrefix string                                     `json:"container_name_prefix"`
		Overrides           map[string]*docker_manager.ServiceOverride `json:"overrides"`
	}

	request := &Request{}
	if err := c.BodyParser(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	manager := docker_manager.GetDockerManager()
	filteredOverrides := make(map[string]*docker_manager.ServiceOverride)
	for name, override := range request.Overrides {
		if _, ok := manager.GetService(name); !ok || override == nil {
			continue
		}
		filteredOverrides[name] = override
	}

	settings := &docker_manager.ServiceSettings{
		ContainerNamePrefix: request.ContainerNamePrefix,
		Overrides:           filteredOverrides,
	}

	if err := manager.SaveServiceSettings(settings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	go manager.ReapplyServiceSettings()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"settings": manager.GetServiceSettings(),
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
	list, starred := manager.Virtualhost.VirtualHostsWithStarred()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":   false,
		"msg":     nil,
		"data":    list,
		"starred": starred,
	})
}

// StarVHost adds a virtual host to the starred list
// @Description Star a virtual host to show it at the top of the list
// @Summary Star a virtual host
// @Tags VirtualHost
// @Accept json
// @Produce json
// @Param path body string true "Path to virtual host configuration file"
// @Success 200 {object} fiber.Map
// @Router /v1/docker/star_vhost [post]
func StarVHost(c *fiber.Ctx) error {
	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	manager := docker_manager.GetDockerManager()
	err := manager.Virtualhost.StarVHost(model.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"starred": true,
		},
	})
}

// UnstarVHost removes a virtual host from the starred list
// @Description Unstar a virtual host
// @Summary Unstar a virtual host
// @Tags VirtualHost
// @Accept json
// @Produce json
// @Param path body string true "Path to virtual host configuration file"
// @Success 200 {object} fiber.Map
// @Router /v1/docker/unstar_vhost [post]
func UnstarVHost(c *fiber.Ctx) error {
	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	manager := docker_manager.GetDockerManager()
	err := manager.Virtualhost.UnstarVHost(model.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"starred": false,
		},
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

// GetVHostEnvMode detects the current environment mode (dev/prod) from virtual host configuration.
// @Description Detect environment mode from virtual host config
// @Summary Get virtual host environment mode
// @Tags VirtualHost
// @Accept json
// @Produce json
// @Param path body string true "Path to virtual host configuration file"
// @Success 200 {object} fiber.Map
// @Router /v1/docker/vhost_env_mode [post]
func GetVHostEnvMode(c *fiber.Ctx) error {
	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	content, err := os.ReadFile(model.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	mode, hasEnvConfig := detectEnvMode(string(content))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"mode":         mode,
			"hasEnvConfig": hasEnvConfig,
		},
	})
}

// ToggleVHostEnvMode toggles between development and production mode in virtual host configuration.
// @Description Toggle environment mode in virtual host config
// @Summary Toggle virtual host environment mode
// @Tags VirtualHost
// @Accept json
// @Produce json
// @Param path body string true "Path to virtual host configuration file"
// @Success 200 {object} fiber.Map
// @Router /v1/docker/toggle_vhost_env [post]
func ToggleVHostEnvMode(c *fiber.Ctx) error {
	type Parameter struct {
		Path string `json:"path"`
	}

	model := &Parameter{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	content, err := os.ReadFile(model.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	newContent, newMode := toggleEnvMode(string(content))

	err = os.WriteFile(model.Path, []byte(newContent), 0644)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Restart nginx/httpd to apply changes
	docker_manager.GetDockerManager().RestartAll()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"mode":    newMode,
			"content": newContent,
		},
	})
}

// detectEnvMode detects the current environment mode from config content.
// Returns mode ("dev", "prod", or "") and whether environment config exists.
func detectEnvMode(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	hasDevActive := false
	hasProdActive := false
	hasEnvConfig := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isCommented := strings.HasPrefix(trimmed, "#")
		lowerLine := strings.ToLower(trimmed)

		// Check for fastcgi_param APP_ENV, APPLICATION_ENV, or SetEnv patterns
		if strings.Contains(lowerLine, "fastcgi_param") || strings.Contains(lowerLine, "setenv") {
			if strings.Contains(lowerLine, "app_env") || strings.Contains(lowerLine, "application_env") {
				hasEnvConfig = true
				if !isCommented {
					if strings.Contains(lowerLine, "dev") || strings.Contains(lowerLine, "development") {
						hasDevActive = true
					} else if strings.Contains(lowerLine, "prod") || strings.Contains(lowerLine, "production") {
						hasProdActive = true
					}
				}
			}
		}
	}

	if hasDevActive {
		return "dev", hasEnvConfig
	}
	if hasProdActive {
		return "prod", hasEnvConfig
	}
	return "", hasEnvConfig
}

// toggleEnvMode toggles between dev and prod mode in config content.
// Returns the new content and the new mode.
func toggleEnvMode(content string) (string, string) {
	lines := strings.Split(content, "\n")
	var newLines []string

	// First pass: determine current mode
	currentMode := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isCommented := strings.HasPrefix(trimmed, "#")
		lowerLine := strings.ToLower(trimmed)

		if strings.Contains(lowerLine, "fastcgi_param") || strings.Contains(lowerLine, "setenv") {
			if strings.Contains(lowerLine, "app_env") || strings.Contains(lowerLine, "application_env") {
				if !isCommented {
					if strings.Contains(lowerLine, "dev") || strings.Contains(lowerLine, "development") {
						currentMode = "dev"
					} else if strings.Contains(lowerLine, "prod") || strings.Contains(lowerLine, "production") {
						currentMode = "prod"
					}
					break
				}
			}
		}
	}

	// Determine target mode
	targetMode := "dev"
	if currentMode == "dev" {
		targetMode = "prod"
	}

	// Second pass: apply the toggle
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lowerLine := strings.ToLower(trimmed)

		if strings.Contains(lowerLine, "fastcgi_param") || strings.Contains(lowerLine, "setenv") {
			if strings.Contains(lowerLine, "app_env") || strings.Contains(lowerLine, "application_env") {
				isCommented := strings.HasPrefix(trimmed, "#")
				isDev := strings.Contains(lowerLine, "dev") || strings.Contains(lowerLine, "development")
				isProd := strings.Contains(lowerLine, "prod") || strings.Contains(lowerLine, "production")

				if !isCommented {
					// Currently active line - comment it out
					newLines = append(newLines, "# "+line)
				} else {
					// Commented line - check if it should be uncommented
					if (targetMode == "dev" && isDev) || (targetMode == "prod" && isProd) {
						// Find the position of # in the line and remove "# " or just "#"
						hashIdx := strings.Index(line, "#")
						if hashIdx >= 0 {
							prefix := line[:hashIdx]
							rest := line[hashIdx+1:]
							// Remove leading space after # if present
							if len(rest) > 0 && rest[0] == ' ' {
								rest = rest[1:]
							}
							newLines = append(newLines, prefix+rest)
						} else {
							newLines = append(newLines, line)
						}
					} else {
						// Keep as commented
						newLines = append(newLines, line)
					}
				}
				continue
			}
		}
		newLines = append(newLines, line)
	}

	return strings.Join(newLines, "\n"), targetMode
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
