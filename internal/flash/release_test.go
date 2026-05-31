package flash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTest overrides package vars for testing; returns a teardown func.
func setupTest(t *testing.T, apiBase string) {
	t.Helper()
	origBase := githubAPIBase
	origOwner := repoOwner
	origRepo := repoName
	origCache := cacheDir
	githubAPIBase = apiBase
	repoOwner = "test-owner"
	repoName = "test-repo"
	cacheDir = t.TempDir()
	t.Cleanup(func() {
		githubAPIBase = origBase
		repoOwner = origOwner
		repoName = origRepo
		cacheDir = origCache
	})
}

func makeServer(t *testing.T, extraAssets string, serveDownload bool) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/test-owner/test-repo/releases/latest" ||
			r.URL.Path == "/repos/test-owner/test-repo/releases/tags/v1.0.0":
			w.Header().Set("Content-Type", "application/json")
			assets := `{"name":"taipanminer-test-board-factory.bin","browser_download_url":"` + srv.URL + `/factory.bin"},` +
				`{"name":"taipanminer-test-board.bin","browser_download_url":"` + srv.URL + `/ota.bin"}`
			if extraAssets != "" {
				assets += "," + extraAssets
			}
			_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[%s]}`, assets)
		case r.URL.Path == "/factory.bin" && serveDownload:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "mock factory firmware content")
		case r.URL.Path == "/ota.bin" && serveDownload:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "mock ota firmware content")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadFirmware_Latest_Factory(t *testing.T) {
	srv := makeServer(t, "", true)
	setupTest(t, srv.URL)

	asset, err := DownloadFirmware("test-board", true, "")
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "taipanminer-test-board-factory.bin", asset.Name)
	assert.Equal(t, "v1.0.0", asset.Version)
	assert.NotEmpty(t, asset.Path)

	content, err := os.ReadFile(asset.Path)
	require.NoError(t, err)
	assert.Equal(t, "mock factory firmware content", string(content))
}

func TestDownloadFirmware_Latest_OTA(t *testing.T) {
	srv := makeServer(t, "", true)
	setupTest(t, srv.URL)

	asset, err := DownloadFirmware("test-board", false, "")
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "taipanminer-test-board.bin", asset.Name)
	assert.Equal(t, "v1.0.0", asset.Version)
	assert.NotEmpty(t, asset.Path)
}

func TestDownloadFirmware_ExplicitVersion_HitsTagsEndpoint(t *testing.T) {
	reqPaths := make([]string, 0)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPaths = append(reqPaths, r.URL.Path)
		switch r.URL.Path {
		case "/repos/test-owner/test-repo/releases/tags/v2.0.0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"taipanminer-test-board-factory.bin","browser_download_url":"%s/fw.bin"}]}`, srv.URL)
		case "/fw.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "v2 firmware")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	asset, err := DownloadFirmware("test-board", true, "v2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", asset.Version)

	// Must not have hit /releases/latest
	for _, p := range reqPaths {
		assert.NotContains(t, p, "latest", "should not hit /latest endpoint when version is set")
	}
}

func TestDownloadFirmware_BoardNotFound_ListsAvailableBoards(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[
			{"name":"taipanminer-alpha-factory.bin","browser_download_url":"%s/fw.bin"},
			{"name":"taipanminer-beta.bin","browser_download_url":"%s/fw.bin"},
			{"name":"taipanminer-gamma-factory.bin","browser_download_url":"%s/fw.bin"},
			{"name":"taipanminer-gamma.bin","browser_download_url":"%s/fw.bin"}
		]}`, srv.URL, srv.URL, srv.URL, srv.URL)
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("unknown-board", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `board "unknown-board" not available in release v1.0.0`)
	assert.Contains(t, err.Error(), "alpha")
	assert.Contains(t, err.Error(), "beta")
	assert.Contains(t, err.Error(), "gamma")
	// boards must be sorted
	assert.Contains(t, err.Error(), "available boards: alpha, beta, gamma")
}

func TestDownloadFirmware_OTAExistsSuggestsSwap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[{"name":"taipanminer-test-board.bin","browser_download_url":"%s/fw.bin"}]}`, srv.URL)
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("test-board", true, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OTA image exists")
	assert.Contains(t, err.Error(), "--ota")
}

func TestDownloadFirmware_FactoryExistsSuggestsSwap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[{"name":"taipanminer-test-board-factory.bin","browser_download_url":"%s/fw.bin"}]}`, srv.URL)
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("test-board", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "factory image exists")
	assert.Contains(t, err.Error(), "omit --ota")
}

func TestDownloadFirmware_NoReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("test-board", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no releases found")
}

func TestDownloadFirmware_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("test-board", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github api returned status 500")
}

func TestDownloadFirmware_DigestPresent_Verified(t *testing.T) {
	content := "verified firmware content"
	h := sha256.Sum256([]byte(content))
	digest := "sha256:" + hex.EncodeToString(h[:])

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test-owner/test-repo/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[{"name":"taipanminer-test-board-factory.bin","browser_download_url":"%s/fw.bin","digest":"%s"}]}`, srv.URL, digest)
		case "/fw.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	asset, err := DownloadFirmware("test-board", true, "")
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(h[:]), asset.SHA256)
}

func TestDownloadFirmware_DigestMismatch_Error(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test-owner/test-repo/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[{"name":"taipanminer-test-board-factory.bin","browser_download_url":"%s/fw.bin","digest":"sha256:deadbeef"}]}`, srv.URL)
		case "/fw.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = fmt.Fprint(w, "actual firmware content")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	setupTest(t, srv.URL)

	_, err := DownloadFirmware("test-board", true, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sha256 mismatch")
}
