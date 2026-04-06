package flash

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ReleaseAsset contains info about a downloaded release asset
type ReleaseAsset struct {
	Name    string
	Version string
	Path    string // local file path after download
}

// Package-level variables for GitHub API configuration
var (
	githubAPIBase = "https://api.github.com"
	repoOwner     = "dangernoodle-io"
	repoName      = "TaipanMiner"
)

// Internal types for GitHub API response
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// DownloadLatestFirmware downloads the latest firmware for a given board from GitHub releases.
// It looks for an asset matching the board name pattern.
// Returns the path to the downloaded file in a temp directory.
func DownloadLatestFirmware(board string) (*ReleaseAsset, error) {
	// Fetch latest release
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBase, repoOwner, repoName)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("no releases found")
	}

	// Find matching asset for board
	var matchedAsset *githubAsset
	for i := range release.Assets {
		if strings.Contains(release.Assets[i].Name, board) {
			matchedAsset = &release.Assets[i]
			break
		}
	}

	if matchedAsset == nil {
		// Build list of available assets
		var available []string
		for _, asset := range release.Assets {
			available = append(available, asset.Name)
		}
		return nil, fmt.Errorf("no asset found for board '%s'; available assets: %v", board, available)
	}

	// Create temp directory and download asset
	tmpDir, err := os.MkdirTemp("", "taipan-firmware-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	localPath := filepath.Join(tmpDir, matchedAsset.Name)
	if err := downloadFile(localPath, matchedAsset.BrowserDownloadURL); err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}

	return &ReleaseAsset{
		Name:    matchedAsset.Name,
		Version: release.TagName,
		Path:    localPath,
	}, nil
}

// downloadFile downloads a file from the given URL and saves it to the given path
func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
