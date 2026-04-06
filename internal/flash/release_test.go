package flash

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadLatestFirmware_Success(t *testing.T) {
	// Mock GitHub API server - create with mutable handler
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test-owner/test-repo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [
					{
						"name": "test-firmware_v1.0.0_test-board.bin",
						"browser_download_url": "%s/firmware.bin"
					}
				]
			}`, server.URL)
			return
		}
		if r.URL.Path == "/firmware.bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "mock firmware binary content")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Override API configuration
	oldBase := githubAPIBase
	oldOwner := repoOwner
	oldRepo := repoName
	githubAPIBase = server.URL
	repoOwner = "test-owner"
	repoName = "test-repo"
	defer func() {
		githubAPIBase = oldBase
		repoOwner = oldOwner
		repoName = oldRepo
	}()

	asset, err := DownloadLatestFirmware("test-board")
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "test-firmware_v1.0.0_test-board.bin", asset.Name)
	assert.Equal(t, "v1.0.0", asset.Version)
	assert.NotEmpty(t, asset.Path)

	// Verify file exists and has content
	content, err := os.ReadFile(asset.Path)
	require.NoError(t, err)
	assert.Equal(t, "mock firmware binary content", string(content))

	// Cleanup
	tmpDir := filepath.Dir(asset.Path)
	_ = os.RemoveAll(tmpDir)
}

func TestDownloadLatestFirmware_NoMatchingAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"tag_name": "v1.0.0",
			"assets": [
				{
					"name": "other-firmware_v1.0.0_other-board.bin",
					"browser_download_url": "http://example.com/other.bin"
				},
				{
					"name": "firmware_v1.0.0_different-board.bin",
					"browser_download_url": "http://example.com/different.bin"
				}
			]
		}`)
	}))
	defer server.Close()

	oldBase := githubAPIBase
	oldOwner := repoOwner
	oldRepo := repoName
	githubAPIBase = server.URL
	repoOwner = "test-owner"
	repoName = "test-repo"
	defer func() {
		githubAPIBase = oldBase
		repoOwner = oldOwner
		repoName = oldRepo
	}()

	asset, err := DownloadLatestFirmware("test-board")
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no asset found for board 'test-board'")
	assert.Contains(t, err.Error(), "other-firmware_v1.0.0_other-board.bin")
	assert.Contains(t, err.Error(), "firmware_v1.0.0_different-board.bin")
}

func TestDownloadLatestFirmware_NoReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"tag_name": "v1.0.0",
			"assets": []
		}`)
	}))
	defer server.Close()

	oldBase := githubAPIBase
	oldOwner := repoOwner
	oldRepo := repoName
	githubAPIBase = server.URL
	repoOwner = "test-owner"
	repoName = "test-repo"
	defer func() {
		githubAPIBase = oldBase
		repoOwner = oldOwner
		repoName = oldRepo
	}()

	asset, err := DownloadLatestFirmware("test-board")
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no releases found")
}

func TestDownloadLatestFirmware_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	oldBase := githubAPIBase
	oldOwner := repoOwner
	oldRepo := repoName
	githubAPIBase = server.URL
	repoOwner = "test-owner"
	repoName = "test-repo"
	defer func() {
		githubAPIBase = oldBase
		repoOwner = oldOwner
		repoName = oldRepo
	}()

	asset, err := DownloadLatestFirmware("test-board")
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "github api returned status 500")
}

func TestDownloadLatestFirmware_DownloadError(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test-owner/test-repo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [
					{
						"name": "test-firmware_v1.0.0_test-board.bin",
						"browser_download_url": "%s/firmware.bin"
					}
				]
			}`, server.URL)
			return
		}
		if r.URL.Path == "/firmware.bin" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	oldBase := githubAPIBase
	oldOwner := repoOwner
	oldRepo := repoName
	githubAPIBase = server.URL
	repoOwner = "test-owner"
	repoName = "test-repo"
	defer func() {
		githubAPIBase = oldBase
		repoOwner = oldOwner
		repoName = oldRepo
	}()

	asset, err := DownloadLatestFirmware("test-board")
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download asset")
	assert.Contains(t, err.Error(), "download returned status 404")
}
