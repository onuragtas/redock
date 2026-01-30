package selfupdate

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// UpdateConfig represents the update configuration from remote JSON
type UpdateConfig struct {
	MinimumRequiredVersion string `json:"minimum_required_version"` // Required: Force update threshold (if current < this → force update)
	ReleaseNotes           string `json:"release_notes"`            // Optional: Brief description
	CriticalUpdate         bool   `json:"critical_update"`          // Optional: Show critical warning in logs
}

// FetchUpdateConfig fetches update configuration from remote JSON
// Primary: GitHub Pages (fast, no rate limit)
// Fallback: GitHub API (if primary fails)
func FetchUpdateConfig(primaryURL, fallbackRepo string) (*UpdateConfig, error) {
	// Try primary source first (GitHub Pages)
	config, err := fetchFromURL(primaryURL)
	if err == nil && config != nil {
		return config, nil
	}

	log.Printf("⚠️  Failed to fetch from primary source: %v", err)

	// Fallback to GitHub API
	return fetchFromGitHubAPI(fallbackRepo)
}

// fetchFromURL fetches JSON from a URL
func fetchFromURL(url string) (*UpdateConfig, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var config UpdateConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// fetchFromGitHubAPI fetches from GitHub releases API as fallback
func fetchFromGitHubAPI(repo string) (*UpdateConfig, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release LastRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}

	// Parse minimum version from release body (if exists)
	minimumVersion := ParseMinimumVersion(release.Body)
	if minimumVersion == "" {
		minimumVersion = "1.0.0" // Default
	}

	config := &UpdateConfig{
		MinimumRequiredVersion: minimumVersion,
		ReleaseNotes:           release.Body,
		CriticalUpdate:         minimumVersion != "1.0.0", // Auto-detect if force update
	}

	return config, nil
}
