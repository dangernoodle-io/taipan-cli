package flash

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCachedFirmware_MissThenHit(t *testing.T) {
	tmp := t.TempDir()
	origCache := cacheDir
	cacheDir = tmp
	t.Cleanup(func() { cacheDir = origCache })

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = fmt.Fprint(w, "firmware bytes")
	}))
	t.Cleanup(srv.Close)

	// First call: cache miss → download
	path1, err := getCachedFirmware("v1.0.0", "fw.bin", srv.URL+"/fw.bin", "")
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&reqCount))

	content, err := os.ReadFile(path1)
	require.NoError(t, err)
	assert.Equal(t, "firmware bytes", string(content))

	// Second call: cache hit → no HTTP request
	path2, err := getCachedFirmware("v1.0.0", "fw.bin", srv.URL+"/fw.bin", "")
	require.NoError(t, err)
	assert.Equal(t, path1, path2)
	assert.Equal(t, int32(1), atomic.LoadInt32(&reqCount), "no second HTTP request expected")
}

func TestGetCachedFirmware_OldMtimeIsReused(t *testing.T) {
	tmp := t.TempDir()
	origCache := cacheDir
	cacheDir = tmp
	t.Cleanup(func() { cacheDir = origCache })

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = fmt.Fprint(w, "old firmware bytes")
	}))
	t.Cleanup(srv.Close)

	// Pre-populate cache with an old mtime (>24h)
	destPath := filepath.Join(tmp, "firmware", "v1.0.0", "fw.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(destPath), 0o755))
	require.NoError(t, os.WriteFile(destPath, []byte("old firmware bytes"), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(destPath, old, old))

	// Should reuse regardless of age when no sha256 check
	path, err := getCachedFirmware("v1.0.0", "fw.bin", srv.URL+"/fw.bin", "")
	require.NoError(t, err)
	assert.Equal(t, destPath, path)
	assert.Equal(t, int32(0), atomic.LoadInt32(&reqCount), "should not re-download even with old mtime")
}

func TestGetCachedFirmware_SHA256Mismatch_Redownload(t *testing.T) {
	tmp := t.TempDir()
	origCache := cacheDir
	cacheDir = tmp
	t.Cleanup(func() { cacheDir = origCache })

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = fmt.Fprint(w, "corrupt data")
	}))
	t.Cleanup(srv.Close)

	// Pre-populate with wrong content
	destPath := filepath.Join(tmp, "firmware", "v1.0.0", "fw.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(destPath), 0o755))
	require.NoError(t, os.WriteFile(destPath, []byte("wrong content"), 0o644))

	// Providing a sha256 that doesn't match the cached file
	// The server also returns corrupt data so sha256 still won't match → error
	_, err := getCachedFirmware("v1.0.0", "fw.bin", srv.URL+"/fw.bin", "aaaa1234")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sha256 mismatch")
	// Verify it attempted a download (removed cached file and tried again)
	assert.Equal(t, int32(1), atomic.LoadInt32(&reqCount))
}

func TestPruneCache_RemovesOtherTagOldFiles(t *testing.T) {
	tmp := t.TempDir()
	origCache := cacheDir
	cacheDir = tmp
	t.Cleanup(func() { cacheDir = origCache })

	// Create old firmware for "v0.9.0" (different tag)
	oldDir := filepath.Join(tmp, "firmware", "v0.9.0")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	oldFile := filepath.Join(oldDir, "fw.bin")
	require.NoError(t, os.WriteFile(oldFile, []byte("old fw"), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(oldFile, old, old))

	// Create firmware for requested tag "v1.0.0" (should NOT be pruned)
	keepDir := filepath.Join(tmp, "firmware", "v1.0.0")
	require.NoError(t, os.MkdirAll(keepDir, 0o755))
	keepFile := filepath.Join(keepDir, "fw.bin")
	require.NoError(t, os.WriteFile(keepFile, []byte("keep fw"), 0o644))
	require.NoError(t, os.Chtimes(keepFile, old, old))

	pruneCache("v1.0.0")

	// Old tag files should be gone
	_, err := os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err), "old firmware file should be deleted")
	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err), "empty old tag dir should be deleted")

	// Requested tag files should remain
	_, err = os.Stat(keepFile)
	assert.NoError(t, err, "requested tag firmware should remain")
}

func TestPruneCache_KeepsRequestedTagRegardlessOfAge(t *testing.T) {
	tmp := t.TempDir()
	origCache := cacheDir
	cacheDir = tmp
	t.Cleanup(func() { cacheDir = origCache })

	keepDir := filepath.Join(tmp, "firmware", "v1.0.0")
	require.NoError(t, os.MkdirAll(keepDir, 0o755))
	keepFile := filepath.Join(keepDir, "fw.bin")
	require.NoError(t, os.WriteFile(keepFile, []byte("keep fw"), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(keepFile, old, old))

	pruneCache("v1.0.0")

	_, err := os.Stat(keepFile)
	assert.NoError(t, err, "requested tag firmware must not be pruned")
}
