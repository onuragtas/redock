package controllers

import (
	"log"
	"redock/backup"
	dockermanager "redock/docker-manager"
	"redock/pkg/utils"
	"redock/selfupdate"

	"github.com/gofiber/fiber/v2"
)

// BackupGetConfig returns the persisted backup tunables (e.g. MaxBackups).
func BackupGetConfig(c *fiber.Ctx) error {
	cfg, err := backup.LoadConfig()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  cfg,
	})
}

// BackupUpdateConfig replaces the on-disk config and immediately re-applies
// the (possibly lowered) retention limit so the user doesn't have to wait
// for the next Create to see the new limit take effect.
func BackupUpdateConfig(c *fiber.Ctx) error {
	cfg := backup.Config{}
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if cfg.MaxBackups < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "max_backups must be >= 1",
		})
	}
	if err := backup.SaveConfig(cfg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if _, err := backup.Prune(cfg.MaxBackups); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "config saved but prune failed: " + err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Config updated",
		"data":  cfg,
	})
}

// BackupList returns all backups in $HOME/redock_backup, newest first.
func BackupList(c *fiber.Ctx) error {
	infos, err := backup.List()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  infos,
	})
}

// BackupCreate makes a new backup of the data directory. Optional body
// {"reason": "..."} is recorded in the manifest; defaults to "manual".
func BackupCreate(c *fiber.Ctx) error {
	type req struct {
		Reason string `json:"reason"`
	}
	body := req{Reason: "manual"}
	_ = c.BodyParser(&body)
	if body.Reason == "" {
		body.Reason = "manual"
	}

	dm := dockermanager.GetDockerManager()
	info, err := backup.Create(dm.GetWorkDir(), currentVersion, body.Reason)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Backup created",
		"data":  info,
	})
}

// BackupDelete removes a backup by ID.
func BackupDelete(c *fiber.Ctx) error {
	type req struct {
		ID string `json:"id"`
	}
	body := req{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if body.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Backup ID is required",
		})
	}
	if err := backup.Delete(body.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Backup deleted",
	})
}

// BackupDownload streams a backup archive to the client as an attachment.
// The ID arrives via query string so a normal <a href> download with a
// pre-signed URL would also work — we still require JWT here and the
// frontend uses an authenticated blob fetch.
func BackupDownload(c *fiber.Ctx) error {
	id := c.Query("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "id query parameter is required",
		})
	}
	path, err := backup.Path(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Download(path, id+".tar.gz")
}

// BackupUpload accepts a multipart "file" field with a redock-created backup
// archive, validates it, and adds it to BackupDir. The manifest's ID
// determines the on-disk filename, so re-uploading the same backup is
// rejected as a duplicate.
func BackupUpload(c *fiber.Ctx) error {
	header, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "file field is required (multipart): " + err.Error(),
		})
	}
	f, err := header.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "open upload: " + err.Error(),
		})
	}
	defer f.Close()

	info, err := backup.Import(f)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Backup imported",
		"data":  info,
	})
}

// BackupRestore replaces the current data dir with the contents of the given
// backup, then restarts the redock process so init.go reloads all state from
// disk. The restore itself runs after graceful Fiber shutdown so HTTP
// handlers don't observe a half-restored state.
func BackupRestore(c *fiber.Ctx) error {
	type req struct {
		ID string `json:"id"`
	}
	body := req{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if body.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Backup ID is required",
		})
	}
	// Existence check up front so we can return a clean error before triggering shutdown.
	if _, err := backup.Path(body.ID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	dm := dockermanager.GetDockerManager()
	id := body.ID
	if utils.RequestGracefulShutdown == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "graceful shutdown unavailable; restore would corrupt running state",
		})
	}
	utils.RequestGracefulShutdown(func() {
		log.Printf("backup: restoring %s into %s", id, dm.GetWorkDir())
		if err := backup.Restore(id, dm.GetWorkDir(), currentVersion); err != nil {
			// We have already shut down; the server is dead in the water.
			// Log loudly so the operator can see the failure in journalctl /
			// terminal output and intervene (e.g. extract by hand or undo
			// from the .pre-restore safety snapshot).
			log.Printf("❌ backup: restore %s failed: %v", id, err)
			return
		}
		log.Printf("backup: restore %s succeeded; restarting process", id)
		if err := selfupdate.RestartProcess(); err != nil {
			log.Printf("❌ backup: restart after restore failed: %v -- redock must be started manually", err)
		}
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Restore started. Server will restart shortly.",
		"data":  fiber.Map{"id": id, "estimated_restart": "10-30 seconds"},
	})
}
