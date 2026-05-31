package flash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// ReleaseAsset contains info about a downloaded release asset
type ReleaseAsset struct {
	Name    string
	Version string
	Path    string   // local file path (from cache)
	SHA256  string   // lowercase hex sha256 (empty if not provided)
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
	Digest             string `json:"digest"`
}

var boardAssetRe = regexp.MustCompile(`^taipanminer-(.+?)(?:-factory)?\.bin$`)

// DownloadFirmware downloads firmware for a given board from GitHub releases.
// If version is empty, the latest release is used.
// If factory is true, downloads taipanminer-<board>-factory.bin; otherwise taipanminer-<board>.bin.
// Returns a ReleaseAsset with the cached file path and resolved tag.
func DownloadFirmware(board string, factory bool, version string) (*ReleaseAsset, error) {
	// Resolve release metadata (possibly from cache)
	release, err := fetchRelease(version)
	if err != nil {
		return nil, err
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

	// Find exact match for expected asset; collect available boards
	var matchedAsset *githubAsset
	var otherTypeExists bool
	boardSet := map[string]struct{}{}
	for i := range release.Assets {
		if m := boardAssetRe.FindStringSubmatch(release.Assets[i].Name); m != nil {
			boardSet[m[1]] = struct{}{}
		}
		if release.Assets[i].Name == expectedName {
			matchedAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == otherName {
			otherTypeExists = true
		}
	}

	if matchedAsset == nil {
		boards := make([]string, 0, len(boardSet))
		for b := range boardSet {
			boards = append(boards, b)
		}
		sort.Strings(boards)
		errMsg := fmt.Sprintf("board %q not available in release %s; available boards: %s",
			board, release.TagName, strings.Join(boards, ", "))
		if otherTypeExists {
			if factory {
				errMsg += " — the OTA image exists, pass --ota to use it"
			} else {
				errMsg += " — the factory image exists, omit --ota to use it"
			}
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Parse sha256 from digest field (format: "sha256:<hex>")
	sha256 := ""
	if d := matchedAsset.Digest; strings.HasPrefix(d, "sha256:") {
		sha256 = strings.TrimPrefix(d, "sha256:")
	}

	// Get-or-download via cache
	path, err := getCachedFirmware(release.TagName, matchedAsset.Name, matchedAsset.BrowserDownloadURL, sha256)
	if err != nil {
		return nil, err
	}

	return &ReleaseAsset{
		Name:    matchedAsset.Name,
		Version: release.TagName,
		Path:    path,
		SHA256:  sha256,
	}, nil
}

// fetchRelease fetches release metadata from GitHub, using cache when possible.
func fetchRelease(version string) (*githubRelease, error) {
	var apiURL string
	cacheKey := version
	if version == "" {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBase, repoOwner, repoName)
		cacheKey = "latest"
	} else {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", githubAPIBase, repoOwner, repoName, version)
	}

	// Try metadata cache
	if data, ok := loadReleaseMeta(cacheKey); ok {
		var release githubRelease
		if err := json.Unmarshal(data, &release); err == nil {
			return &release, nil
		}
	}

	// Fetch from GitHub
	resp, err := http.Get(apiURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	// Decode
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	// Cache encoded form
	if data, err := json.Marshal(&release); err == nil {
		_ = storeReleaseMeta(cacheKey, data)
	}

	return &release, nil
}
