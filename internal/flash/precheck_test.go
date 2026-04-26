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

func TestPrecheck_ForcedSkipsAll(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "invalid.bin")
	// Create an invalid file
	require.NoError(t, os.WriteFile(binPath, []byte("bad"), 0o644))

	// force=true should skip all checks and not error
	err := Precheck("bitaxe-601", binPath, "", "", false, true)
	require.NoError(t, err)
}

func TestPrecheck_ProjectNameMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	// Create firmware with mismatched project name
	binData := buildTestBinary(t, "taipanminer-bitaxe-403", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// Try to flash to different board
	err := Precheck("bitaxe-601", binPath, "", "", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain board")
}

func TestPrecheck_BinarySizeOversized_OTA(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "oversized.bin")

	// Create an oversized OTA binary with valid app descriptor
	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	oversized := make([]byte, ota0SlotSize+1)
	copy(oversized, binData)
	require.NoError(t, os.WriteFile(binPath, oversized, 0o644))

	err := Precheck("bitaxe-601", binPath, "", "", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds ota_0 slot size")
}

func TestPrecheck_BinarySizeOversized_Factory(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "oversized_factory.bin")

	// Create a factory binary where the app slice (from 0x20000 to EOF) is oversized
	binData := buildTestFactoryBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	oversized := make([]byte, 0x20000+ota0SlotSize+1)
	copy(oversized, binData)
	require.NoError(t, os.WriteFile(binPath, oversized, 0o644))

	err := Precheck("bitaxe-601", binPath, "", "", true, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app slice size")
	assert.Contains(t, err.Error(), "exceeds ota_0 slot size")
}

func TestPrecheck_DeviceViaHTTP_Success(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"board":"bitaxe-601"}`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := Precheck("bitaxe-601", binPath, server.Listener.Addr().String(), "", false, false)
	require.NoError(t, err)
}

func TestPrecheck_DeviceViaHTTP_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// Mock HTTP server with mismatched board
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"board":"bitdsk-n8t"}`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := Precheck("bitaxe-601", binPath, server.Listener.Addr().String(), "", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match --board")
}

func TestPrecheck_ValidFirmwareNoHost(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// No host means serial flash path; force=true to skip the probe check
	err := Precheck("bitaxe-601", binPath, "", "", false, true)
	require.NoError(t, err)
}

func TestPrecheck_N8T_Board(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	binData := buildTestBinary(t, "taipanminer-bitdsk-n8t", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// Mock HTTP server for OTA path
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"board":"bitdsk-n8t"}`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := Precheck("bitdsk-n8t", binPath, server.Listener.Addr().String(), "", false, false)
	require.NoError(t, err)
}

func TestPrecheck_Factory_Success(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "factory.bin")

	binData := buildTestFactoryBinary(t, "taipanminer-tdongle-s3", "v1.5.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"board":"tdongle-s3"}`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := Precheck("tdongle-s3", binPath, server.Listener.Addr().String(), "", true, false)
	require.NoError(t, err)
}
