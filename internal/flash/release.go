package flash

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
// If factory is true, downloads taipanminer-<board>-factory.bin (full image);
// otherwise downloads taipanminer-<board>.bin (app-only OTA image).
// Returns the path to the downloaded file in a temp directory.
func DownloadLatestFirmware(board string, factory bool) (*ReleaseAsset, error) {
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

	// Build expected asset name
	var expectedName string
	var otherName string
	if factory {
		expectedName = fmt.Sprintf("taipanminer-%s-factory.bin", board)
		otherName = fmt.Sprintf("taipanminer-%s.bin", board)
	} else {
		expectedName = fmt.Sprintf("taipanminer-%s.bin", board)
		otherName = fmt.Sprintf("taipanminer-%s-factory.bin", board)
	}

	// Find exact match for expected asset
	var matchedAsset *githubAsset
	var otherTypeExists bool
	var available []string
	for i := range release.Assets {
		available = append(available, release.Assets[i].Name)
		if release.Assets[i].Name == expectedName {
			matchedAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == otherName {
			otherTypeExists = true
		}
	}

	if matchedAsset == nil {
		// Build helpful error message
		errMsg := fmt.Sprintf("no asset %q found; available assets: %v", expectedName, available)
		if otherTypeExists {
			if factory {
				errMsg += " — the OTA image exists, pass --ota to use it"
			} else {
				errMsg += " — the factory image exists, omit --ota to use it"
			}
		}
		return nil, fmt.Errorf("%s", errMsg)
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
