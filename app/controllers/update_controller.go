package controllers

import (
	"fmt"
	"log"
	"redock/app/cache_models"
	"redock/platform/database"
	"redock/platform/memory"
	"redock/selfupdate"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	currentVersion = "1.0.0" // Will be overridden by app_version.go
)

// SetCurrentVersion sets the current version (called from main)
func SetCurrentVersion(v string) {
	currentVersion = v
}

// getCachedReleases returns cached releases or fetches from GitHub
func getCachedReleases() ([]selfupdate.ReleaseInfo, error) {
	owner := "onuragtas"
	repo := "redock"
	db := database.GetMemoryDB()
	
	// Find existing cache
	caches := memory.Filter[*cache_models.ReleaseCache](db, "release_cache", func(c *cache_models.ReleaseCache) bool {
		return c.Owner == owner && c.Repo == repo
	})
	
	if len(caches) > 0 {
		cache := caches[0]
		// Check if cache is still valid (5 minutes)
		if cache.IsValid() {
			return cache.Releases, nil
		}
	}
	
	// Cache expired or not found, fetch from GitHub
	releases, err := selfupdate.FetchAllReleases(owner, repo)
	if err != nil {
		// If fetch fails but we have old cache, return it
		if len(caches) > 0 && len(caches[0].Releases) > 0 {
			log.Printf("⚠️  GitHub API failed, using stale cache: %v", err)
			return caches[0].Releases, nil
		}
		return nil, err
	}
	
	// Update or create cache
	if len(caches) > 0 {
		// Update existing cache
		releaseCache := caches[0]
		releaseCache.Releases = releases
		releaseCache.FetchedAt = time.Now()
		if err := memory.Update(db, "release_cache", releaseCache); err != nil {
			log.Printf("⚠️  Failed to update cache: %v", err)
		}
	} else {
		// Create new cache
		releaseCache := &cache_models.ReleaseCache{
			Owner:     owner,
			Repo:      repo,
			Releases:  releases,
			FetchedAt: time.Now(),
		}
		if err := memory.Create(db, "release_cache", releaseCache); err != nil {
			log.Printf("⚠️  Failed to create cache: %v", err)
		}
	}
	
	return releases, nil
}

// GetAvailableUpdates returns available updates for the current version
func GetAvailableUpdates(c *fiber.Ctx) error {
	// Fetch releases from cache (or GitHub if cache expired)
	releases, err := getCachedReleases()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch releases: " + err.Error(),
		})
	}

	// Parse current version
	current, err := selfupdate.ParseVersion(currentVersion)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid current version: " + err.Error(),
		})
	}

	// Filter available updates
	available, err := selfupdate.FilterAvailableUpdates(currentVersion, releases)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to filter updates: " + err.Error(),
		})
	}

	// Format response
	updates := make([]fiber.Map, 0)
	for _, release := range available {
		releaseVer, _ := selfupdate.ParseVersion(release.TagName)
		
		downloadURL := selfupdate.FindAssetForPlatform(release, runtime.GOOS, runtime.GOARCH)
		if downloadURL == "" {
			continue // Skip if no asset for this platform
		}

		updateType := "stable"
		if releaseVer.IsBeta() {
			updateType = "beta"
		}

		// Determine if this is recommended
		recommended := false
		if current.IsStable() && releaseVer.IsStable() && releaseVer.Compare(current) > 0 {
			recommended = true // Next stable is recommended
		}
		if current.IsBeta() && releaseVer.IsStable() &&
			releaseVer.Major == current.Major &&
			releaseVer.Minor == current.Minor &&
			releaseVer.Patch == current.Patch {
			recommended = true // Graduation to stable is recommended
		}

		updates = append(updates, fiber.Map{
			"version":      releaseVer.String(),
			"tag":          release.TagName,
			"type":         updateType,
			"name":         release.Name,
			"description":  release.Body,
			"published_at": release.PublishedAt.Format(time.RFC3339),
			"download_url": downloadURL,
			"recommended":  recommended,
			"pre_release":  release.PreRelease,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":   false,
		"data": fiber.Map{
			"current_version": current.String(),
			"is_beta":         current.IsBeta(),
			"is_stable":       current.IsStable(),
			"updates":         updates,
			"update_count":    len(updates),
		},
	})
}

// ApplyUpdate downloads and applies an update
func ApplyUpdate(c *fiber.Ctx) error {
	var req struct {
		Version string `json:"version"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	if req.Version == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Version is required",
		})
	}

	// Fetch releases to find the requested version
	releases, err := selfupdate.FetchAllReleases("onuragtas", "redock")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch releases: " + err.Error(),
		})
	}

	var targetRelease *selfupdate.ReleaseInfo
	for _, release := range releases {
		if release.TagName == req.Version || "v"+req.Version == release.TagName {
			targetRelease = &release
			break
		}
	}

	if targetRelease == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Version not found: " + req.Version,
		})
	}

	downloadURL := selfupdate.FindAssetForPlatform(*targetRelease, runtime.GOOS, runtime.GOARCH)
	if downloadURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No binary available for your platform",
		})
	}

	// Start update in background
	go func() {
		updater := &selfupdate.Updater{
			CurrentVersion: currentVersion,
			BinURL:         downloadURL,
			Dir:            "update/",
			CmdName:        "/redock",
		}

		if err := updater.UpdateWithRestart(); err != nil {
			log.Printf("❌ Update failed: %v", err)
		}
	}()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   fmt.Sprintf("Update to %s started. Server will restart shortly...", req.Version),
		"data": fiber.Map{
			"version":           req.Version,
			"estimated_restart": "30 seconds",
		},
	})
}

// GetCurrentVersion returns the current version
func GetCurrentVersion(c *fiber.Ctx) error {
	current, err := selfupdate.ParseVersion(currentVersion)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid version format",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"version":   current.String(),
			"raw":       currentVersion,
			"is_beta":   current.IsBeta(),
			"is_stable": current.IsStable(),
		},
	})
}

