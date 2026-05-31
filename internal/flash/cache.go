package flash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	firmwareTTL = 24 * time.Hour
	metaTTL     = time.Hour
)

// cacheDir returns the base cache directory. Overridable in tests.
var cacheDir = func() string {
	d, _ := os.UserCacheDir()
	return filepath.Join(d, "taipan")
}()

// getCachedFirmware returns the local path to the firmware, downloading if needed.
// sha256expected may be empty (skip verification).
func getCachedFirmware(tag, assetName, downloadURL, sha256expected string) (string, error) {
	destPath := filepath.Join(cacheDir, "firmware", tag, assetName)

	// Check if file exists; if sha256 known, verify it
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		if sha256expected == "" {
			// No digest — reuse without verification; refresh mtime
			_ = os.Chtimes(destPath, time.Now(), time.Now())
			return destPath, nil
		}
		actual, err := fileSHA256(destPath)
		if err == nil && actual == sha256expected {
			_ = os.Chtimes(destPath, time.Now(), time.Now())
			return destPath, nil
		}
		// sha256 mismatch — remove and re-download
		_ = os.Remove(destPath)
	}

	// Download
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("cache mkdir: %w", err)
	}
	if err := downloadFileHTTP(destPath, downloadURL); err != nil {
		return "", fmt.Errorf("failed to download asset: %w", err)
	}

	// Verify sha256 after download if known
	if sha256expected != "" {
		actual, err := fileSHA256(destPath)
		if err != nil {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("sha256 check: %w", err)
		}
		if actual != sha256expected {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("sha256 mismatch: got %s, want %s", actual, sha256expected)
		}
	}

	return destPath, nil
}

// loadReleaseMeta reads cached release metadata; returns (data, true) if fresh.
func loadReleaseMeta(key string) ([]byte, bool) {
	path := metaPath(key)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > metaTTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// storeReleaseMeta writes release metadata JSON to the cache.
func storeReleaseMeta(key string, data []byte) error {
	path := metaPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func metaPath(key string) string {
	return filepath.Join(cacheDir, "release", key+".json")
}

// pruneCache removes stale firmware for tags other than requestedTag,
// and stale metadata entries. All errors are non-fatal.
func pruneCache(requestedTag string) {
	firmwareDir := filepath.Join(cacheDir, "firmware")
	entries, err := os.ReadDir(firmwareDir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tag := e.Name()
		if tag == requestedTag {
			continue
		}
		tagDir := filepath.Join(firmwareDir, tag)
		files, err := os.ReadDir(tagDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			fp := filepath.Join(tagDir, f.Name())
			fi, err := os.Stat(fp)
			if err != nil {
				continue
			}
			if now.Sub(fi.ModTime()) > firmwareTTL {
				_ = os.Remove(fp)
			}
		}
		// Remove empty dir
		remaining, _ := os.ReadDir(tagDir)
		if len(remaining) == 0 {
			_ = os.Remove(tagDir)
		}
	}

	// Prune stale metadata (never prune the in-use version or "latest")
	releaseDir := filepath.Join(cacheDir, "release")
	metas, err := os.ReadDir(releaseDir)
	if err != nil {
		return
	}
	inUseKey := requestedTag + ".json"
	for _, m := range metas {
		if m.Name() == inUseKey || m.Name() == "latest.json" {
			continue
		}
		mp := filepath.Join(releaseDir, m.Name())
		fi, err := os.Stat(mp)
		if err != nil {
			continue
		}
		if now.Sub(fi.ModTime()) > metaTTL {
			_ = os.Remove(mp)
		}
	}
}

// downloadFileHTTP downloads from url to destPath.
func downloadFileHTTP(destPath, url string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// fileSHA256 computes the lowercase hex sha256 of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
