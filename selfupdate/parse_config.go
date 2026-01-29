package selfupdate

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ParseMinimumVersion extracts minimum required version from release body
// Supports multiple formats:
// 1. JSON: {"minimum_required_version": "1.0.250"}
// 2. Text: MINIMUM_REQUIRED_VERSION: 1.0.250
// 3. Text: minimum_required_version=1.0.250
func ParseMinimumVersion(body string) string {
	if body == "" {
		return ""
	}

	// Try JSON format first
	var config ReleaseConfig
	if err := json.Unmarshal([]byte(body), &config); err == nil {
		if config.MinimumRequiredVersion != "" {
			return config.MinimumRequiredVersion
		}
	}

	// Try to find JSON block in markdown
	jsonBlockRegex := regexp.MustCompile("```json\\s*([\\s\\S]*?)\\s*```")
	if matches := jsonBlockRegex.FindStringSubmatch(body); len(matches) > 1 {
		var config ReleaseConfig
		if err := json.Unmarshal([]byte(matches[1]), &config); err == nil {
			if config.MinimumRequiredVersion != "" {
				return config.MinimumRequiredVersion
			}
		}
	}

	// Try text formats
	patterns := []string{
		`(?i)MINIMUM[_\s-]REQUIRED[_\s-]VERSION[:\s=]+v?([0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9\.]+)?)`,
		`(?i)min[_\s-]version[:\s=]+v?([0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9\.]+)?)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(body); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}
