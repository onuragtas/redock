package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Version represents a semantic version with optional pre-release info
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string // e.g., "beta.1"
	Raw        string
}

// ReleaseInfo represents a GitHub release
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	PreRelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// ParseVersion parses a version string (e.g., "v1.2.3-beta.1" or "1.2.3")
func ParseVersion(v string) (*Version, error) {
	v = strings.TrimPrefix(v, "v")
	
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9\.]+))?$`)
	matches := re.FindStringSubmatch(v)
	
	if matches == nil {
		return nil, fmt.Errorf("invalid version format: %s", v)
	}
	
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	
	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: matches[4],
		Raw:        v,
	}, nil
}

// IsStable returns true if version is not a pre-release
func (v *Version) IsStable() bool {
	return v.PreRelease == ""
}

// IsBeta returns true if version is a beta release
func (v *Version) IsBeta() bool {
	return strings.HasPrefix(v.PreRelease, "beta")
}

// Compare compares two versions
// Returns: -1 if v < other, 0 if v == other, 1 if v > other
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}
	
	// Same major.minor.patch, compare pre-release
	if v.PreRelease == "" && other.PreRelease != "" {
		return 1 // Stable > Beta
	}
	if v.PreRelease != "" && other.PreRelease == "" {
		return -1 // Beta < Stable
	}
	if v.PreRelease == other.PreRelease {
		return 0
	}
	
	// Both have pre-release, compare beta numbers
	vBeta := extractBetaNumber(v.PreRelease)
	otherBeta := extractBetaNumber(other.PreRelease)
	
	if vBeta > otherBeta {
		return 1
	}
	if vBeta < otherBeta {
		return -1
	}
	
	return 0
}

func extractBetaNumber(preRelease string) int {
	re := regexp.MustCompile(`beta\.(\d+)`)
	matches := re.FindStringSubmatch(preRelease)
	if len(matches) > 1 {
		num, _ := strconv.Atoi(matches[1])
		return num
	}
	return 0
}

// String returns the version string
func (v *Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		s += "-" + v.PreRelease
	}
	return s
}

// FetchAllReleases fetches all releases from GitHub
func FetchAllReleases(owner, repo string) ([]ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(body))
	}
	
	var releases []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	
	return releases, nil
}

// GetLatestBetaVersion fetches the latest beta version from GitHub
func GetLatestBetaVersion(owner, repo string) (string, error) {
	releases, err := FetchAllReleases(owner, repo)
	if err != nil {
		return "", err
	}
	
	// Find the latest beta (first pre-release in the list)
	for _, release := range releases {
		if release.PreRelease {
			ver, err := ParseVersion(release.TagName)
			if err != nil {
				continue
			}
			if ver.IsBeta() {
				return ver.String(), nil
			}
		}
	}
	
	return "", fmt.Errorf("no beta version found")
}

// FilterAvailableUpdates returns available updates based on current version
func FilterAvailableUpdates(currentVersion string, releases []ReleaseInfo) ([]ReleaseInfo, error) {
	current, err := ParseVersion(currentVersion)
	if err != nil {
		return nil, err
	}
	
	var available []ReleaseInfo
	
	for _, release := range releases {
		// Skip drafts
		if release.Draft {
			continue
		}
		
		releaseVer, err := ParseVersion(release.TagName)
		if err != nil {
			continue // Skip invalid versions
		}
		
		// If current is stable
		if current.IsStable() {
			// Show newer stable versions
			if releaseVer.IsStable() && releaseVer.Compare(current) > 0 {
				available = append(available, release)
			}
			// Show beta versions of next minor/major
			if releaseVer.IsBeta() && (
				releaseVer.Major > current.Major ||
				(releaseVer.Major == current.Major && releaseVer.Minor > current.Minor) ||
				(releaseVer.Major == current.Major && releaseVer.Minor == current.Minor && releaseVer.Patch > current.Patch)) {
				available = append(available, release)
			}
		} else if current.IsBeta() {
			// If current is beta, show:
			// 1. Newer beta versions of same base version
			if releaseVer.IsBeta() &&
				releaseVer.Major == current.Major &&
				releaseVer.Minor == current.Minor &&
				releaseVer.Patch == current.Patch &&
				releaseVer.Compare(current) > 0 {
				available = append(available, release)
			}
			
			// 2. Stable version of same or higher version
			if releaseVer.IsStable() && releaseVer.Compare(current) >= 0 {
				available = append(available, release)
			}
			
			// 3. Newer beta versions (higher minor/major)
			if releaseVer.IsBeta() && (
				releaseVer.Major > current.Major ||
				(releaseVer.Major == current.Major && releaseVer.Minor > current.Minor)) {
				available = append(available, release)
			}
		}
	}
	
	return available, nil
}

// FindAssetForPlatform finds the appropriate asset for the current platform
func FindAssetForPlatform(release ReleaseInfo, platform, arch string) string {
	// GitHub releases use lowercase platform names: redock_darwin_amd64
	// No conversion needed - use platform and arch as-is
	
	if platform == "" || arch == "" {
		return ""
	}
	
	// GitHub asset naming: redock_{platform}_{arch}
	assetName := fmt.Sprintf("redock_%s_%s", platform, arch)
	
	// Check if assets exist
	if len(release.Assets) == 0 {
		return ""
	}
	
	for _, asset := range release.Assets {
		// Safe check: ensure Name is not empty
		if asset.Name != "" && asset.Name == assetName {
			return asset.BrowserDownloadURL
		}
	}
	
	return ""
}
