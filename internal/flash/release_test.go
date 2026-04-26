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

func TestDownloadLatestFirmware_Factory(t *testing.T) {
	// Mock GitHub API server with factory asset
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test-owner/test-repo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [
					{
						"name": "taipanminer-test-board-factory.bin",
						"browser_download_url": "%s/firmware.bin"
					},
					{
						"name": "taipanminer-test-board.bin",
						"browser_download_url": "%s/firmware-ota.bin"
					}
				]
			}`, server.URL, server.URL)
			return
		}
		if r.URL.Path == "/firmware.bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "mock factory firmware content")
			return
		}
		w.WriteHeader(http.StatusNotFound)
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

	asset, err := DownloadLatestFirmware("test-board", true)
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "taipanminer-test-board-factory.bin", asset.Name)
	assert.Equal(t, "v1.0.0", asset.Version)
	assert.NotEmpty(t, asset.Path)

	content, err := os.ReadFile(asset.Path)
	require.NoError(t, err)
	assert.Equal(t, "mock factory firmware content", string(content))

	tmpDir := filepath.Dir(asset.Path)
	_ = os.RemoveAll(tmpDir)
}

func TestDownloadLatestFirmware_OTA(t *testing.T) {
	// Mock GitHub API server with OTA asset
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test-owner/test-repo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v1.0.0",
				"assets": [
					{
						"name": "taipanminer-test-board.bin",
						"browser_download_url": "%s/firmware.bin"
					}
				]
			}`, server.URL)
			return
		}
		if r.URL.Path == "/firmware.bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "mock ota firmware content")
			return
		}
		w.WriteHeader(http.StatusNotFound)
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

	asset, err := DownloadLatestFirmware("test-board", false)
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "taipanminer-test-board.bin", asset.Name)
	assert.Equal(t, "v1.0.0", asset.Version)

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
					"name": "taipanminer-other-board.bin",
					"browser_download_url": "http://example.com/other.bin"
				},
				{
					"name": "taipanminer-different-board-factory.bin",
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

	asset, err := DownloadLatestFirmware("test-board", false)
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no asset \"taipanminer-test-board.bin\" found")
	assert.Contains(t, err.Error(), "taipanminer-other-board.bin")
	assert.Contains(t, err.Error(), "taipanminer-different-board-factory.bin")
}

func TestDownloadLatestFirmware_OTAExistsSuggestsSwap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"tag_name": "v1.0.0",
			"assets": [
				{
					"name": "taipanminer-test-board.bin",
					"browser_download_url": "http://example.com/ota.bin"
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

	asset, err := DownloadLatestFirmware("test-board", true)
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no asset \"taipanminer-test-board-factory.bin\" found")
	assert.Contains(t, err.Error(), "OTA image exists")
	assert.Contains(t, err.Error(), "--ota")
}

func TestDownloadLatestFirmware_FactoryExistsSuggestsSwap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"tag_name": "v1.0.0",
			"assets": [
				{
					"name": "taipanminer-test-board-factory.bin",
					"browser_download_url": "http://example.com/factory.bin"
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

	asset, err := DownloadLatestFirmware("test-board", false)
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no asset \"taipanminer-test-board.bin\" found")
	assert.Contains(t, err.Error(), "factory image exists")
	assert.Contains(t, err.Error(), "omit --ota")
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

	asset, err := DownloadLatestFirmware("test-board", false)
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

	asset, err := DownloadLatestFirmware("test-board", false)
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
						"name": "taipanminer-test-board.bin",
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

	asset, err := DownloadLatestFirmware("test-board", false)
	assert.Nil(t, asset)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download asset")
	assert.Contains(t, err.Error(), "download returned status 404")
}
