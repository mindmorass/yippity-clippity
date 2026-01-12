package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// GitHubRepo is the repository to check for updates
	GitHubRepo = "mindmorass/yippity-clippity"

	// CheckInterval is how often to check for updates
	CheckInterval = 6 * time.Hour
)

// Release represents a GitHub release
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ReleaseNotes   string
	PublishedAt    time.Time
}

// Checker handles checking for updates
type Checker struct {
	currentVersion string
	lastCheck      time.Time
	lastResult     *UpdateInfo
	httpClient     *http.Client
}

// NewChecker creates a new update checker
func NewChecker(currentVersion string) *Checker {
	return &Checker{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Check checks for updates from GitHub
func (c *Checker) Check() (*UpdateInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "yippity-clippity-update-checker")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No releases yet
		return &UpdateInfo{
			Available:      false,
			CurrentVersion: c.currentVersion,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	// Skip prereleases and drafts
	if release.Prerelease || release.Draft {
		return &UpdateInfo{
			Available:      false,
			CurrentVersion: c.currentVersion,
		}, nil
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(c.currentVersion, "v")

	info := &UpdateInfo{
		Available:      isNewerVersion(latestVersion, currentVersion),
		CurrentVersion: c.currentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
	}

	c.lastCheck = time.Now()
	c.lastResult = info

	return info, nil
}

// CheckIfNeeded checks for updates if enough time has passed
func (c *Checker) CheckIfNeeded() (*UpdateInfo, error) {
	if time.Since(c.lastCheck) < CheckInterval && c.lastResult != nil {
		return c.lastResult, nil
	}
	return c.Check()
}

// GetLastResult returns the last check result without making a request
func (c *Checker) GetLastResult() *UpdateInfo {
	return c.lastResult
}

// GetCurrentVersion returns the current version
func (c *Checker) GetCurrentVersion() string {
	return c.currentVersion
}

// isNewerVersion compares semantic versions (simple implementation)
// Returns true if latest is newer than current
func isNewerVersion(latest, current string) bool {
	// Handle dev version
	if current == "dev" || current == "" {
		return false // Don't prompt for updates on dev builds
	}

	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}

	return false // Equal versions
}

// parseVersion parses a version string into [major, minor, patch]
func parseVersion(v string) [3]int {
	var parts [3]int
	v = strings.TrimPrefix(v, "v")

	// Remove any suffix like -alpha, -beta, -rc
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	segments := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(segments); i++ {
		fmt.Sscanf(segments[i], "%d", &parts[i])
	}

	return parts
}
