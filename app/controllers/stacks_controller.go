package controllers

import (
	"archive/zip"
	"context"

	docker_manager "redock/docker-manager"
	"redock/docker-manager/stacks"

	"github.com/gofiber/fiber/v2"
)

// stacksManager returns the singleton stacks Manager bound to the docker work dir.
func stacksManager() (*stacks.Manager, error) {
	workDir := docker_manager.GetDockerManager().GetWorkDir()
	return stacks.GetManager(workDir)
}

func stacksErr(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": true, "msg": err.Error(), "data": nil})
}

func stacksOK(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "msg": nil, "data": data})
}

// StacksCatalog returns the merged, env-resolved catalog plus any repo sync errors.
func StacksCatalog(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	specs, syncErrs := m.Catalog()
	errMap := map[string]string{}
	for name, e := range syncErrs {
		errMap[name] = e.Error()
	}
	return stacksOK(c, fiber.Map{"services": specs, "sync_errors": errMap})
}

// StacksStatus returns the runtime status of every catalog service.
func StacksStatus(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	status, err := m.Status(context.Background())
	if err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"status": status})
}

// StacksUp starts the given services (and their dependencies).
func StacksUp(c *fiber.Ctx) error {
	var body struct {
		Services []string `json:"services"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.Up(context.Background(), body.Services...); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"started": body.Services})
}

// StacksDown stops and removes a service.
func StacksDown(c *fiber.Ctx) error {
	var body struct {
		Service string `json:"service"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.Down(context.Background(), body.Service); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"stopped": body.Service})
}

// StacksRestart restarts a service.
func StacksRestart(c *fiber.Ctx) error {
	var body struct {
		Service string `json:"service"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.Restart(context.Background(), body.Service); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"restarted": body.Service})
}

// StacksListRepositories returns the configured repositories.
func StacksListRepositories(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"repositories": m.Registry.Repos})
}

// StacksAddRepository registers a new repository (compose URL or local dir).
func StacksAddRepository(c *fiber.Ctx) error {
	var repo stacks.Repository
	if err := c.BodyParser(&repo); err != nil {
		return stacksErr(c, err)
	}
	if repo.Kind == "" {
		repo.Kind = stacks.RepoComposeURL
	}
	repo.Enabled = true
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.AddRepository(repo); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"repository": repo})
}

// StacksRemoveRepository removes a repository by name.
func StacksRemoveRepository(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.RemoveRepository(body.Name); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"removed": body.Name})
}

// StacksUpdateRepository edits an existing repository's source (kind/location/compose).
func StacksUpdateRepository(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		stacks.Repository
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.UpdateRepository(body.Name, body.Repository); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"repository": body.Name})
}

// StacksToggleRepository enables or disables a repository without removing it.
func StacksToggleRepository(c *fiber.Ctx) error {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.SetRepositoryEnabled(body.Name, body.Enabled); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"name": body.Name, "enabled": body.Enabled})
}

// StacksUploadRepository accepts a multipart zip upload (a stack with a compose
// file + build contexts), extracts it, and registers it as a local repository.
func StacksUploadRepository(c *fiber.Ctx) error {
	name := c.FormValue("name")
	compose := c.FormValue("compose")
	fh, err := c.FormFile("file")
	if err != nil {
		return stacksErr(c, err)
	}
	f, err := fh.Open()
	if err != nil {
		return stacksErr(c, err)
	}
	defer f.Close()
	zr, err := zip.NewReader(f, fh.Size)
	if err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.ImportZipRepository(name, compose, zr); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"repository": name})
}

// StacksSyncRepositories re-fetches all enabled repositories.
func StacksSyncRepositories(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	errMap := map[string]string{}
	for name, e := range m.Sync() {
		errMap[name] = e.Error()
	}
	return stacksOK(c, fiber.Map{"sync_errors": errMap})
}

// StacksAddService registers a single Hub-image service (no repository/build).
func StacksAddService(c *fiber.Ctx) error {
	var body struct {
		stacks.ServiceSpec
		Dockerfile string `json:"dockerfile"`
		Files      []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if body.Dockerfile != "" {
		files := map[string]string{}
		for _, f := range body.Files {
			if f.Path != "" {
				files[f.Path] = f.Content
			}
		}
		if err := m.AddCustomBuildService(body.ServiceSpec, body.Dockerfile, files); err != nil {
			return stacksErr(c, err)
		}
	} else if err := m.AddCustomService(body.ServiceSpec); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"service": body.Name})
}

// StacksUpdateService edits an existing custom Hub-image service in place.
func StacksUpdateService(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		stacks.ServiceSpec
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	if body.Name == "" {
		body.Name = body.ServiceSpec.Name
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.UpdateCustomService(body.Name, body.ServiceSpec); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"service": body.Name})
}

// StacksRemoveService removes a custom Hub-image service.
func StacksRemoveService(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.RemoveCustomService(body.Name); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"removed": body.Name})
}

// StacksGetEnv returns the structured environment (repo defaults + global
// overrides): each variable with its effective value, default, and override flag.
func StacksGetEnv(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"vars": m.EnvVars()})
}

// StacksSaveEnv persists the global .env overrides from a key→value map.
func StacksSaveEnv(c *fiber.Ctx) error {
	var body struct {
		Vars map[string]string `json:"vars"`
	}
	if err := c.BodyParser(&body); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.SaveEnvVars(body.Vars); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"saved": true})
}

// StacksGetDevEnvSettings returns the configurable dev-environment parameters.
func StacksGetDevEnvSettings(c *fiber.Ctx) error {
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"settings": m.GetDevEnvSettings()})
}

// StacksSaveDevEnvSettings persists the dev-environment parameters.
func StacksSaveDevEnvSettings(c *fiber.Ctx) error {
	var s stacks.DevEnvSettings
	if err := c.BodyParser(&s); err != nil {
		return stacksErr(c, err)
	}
	m, err := stacksManager()
	if err != nil {
		return stacksErr(c, err)
	}
	if err := m.SaveDevEnvSettings(s); err != nil {
		return stacksErr(c, err)
	}
	return stacksOK(c, fiber.Map{"settings": s})
}

// Note: virtual-host and XDebug management are NOT exposed as separate stacks
// endpoints. The existing Virtual Hosts page and dashboard XDebug action drive
// the stacks engine transparently: docker_manager writes the config files into
// the shared dirs (etc/nginx, httpd/sites-enabled) that stacks nginx/httpd
// mount, and delegates container restarts + xdebug.ini regeneration to stacks.
