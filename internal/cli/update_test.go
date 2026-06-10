package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/ota"
)

func parseTestHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return u.Hostname(), p
}

// reportBootedVersion should not panic when the device is unreachable and
// the fallback "(expected ...)" message is emitted.
func TestReportBootedVersion_Unreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	client := ota.NewClient("127.0.0.1", port)
	// Call returns cleanly without panicking; output goes to stdout.
	reportBootedVersion(client, "test-host", "v0.7.5")
}

// reportBootedVersion should log a success when the device reports the
// expected version.
func TestReportBootedVersion_MatchingVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			_, _ = w.Write([]byte(`{"version":"v0.7.5"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	client := ota.NewClient(host, port)
	reportBootedVersion(client, "test-host", "v0.7.5")
}

// reportBootedVersion should handle a mismatched booted version gracefully.
func TestReportBootedVersion_Mismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			_, _ = w.Write([]byte(`{"version":"v0.7.4"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	client := ota.NewClient(host, port)
	reportBootedVersion(client, "test-host", "v0.7.5")
}

// isNetworkError returns true for connection-refused style errors.
func TestIsNetworkError_NetErr(t *testing.T) {
	// Dial a closed port to produce a net.OpError with ECONNREFUSED.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	_, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	require.Error(t, err)
	assert.True(t, isNetworkError(err))
}

// isNetworkError returns false for nil or non-network errors.
func TestIsNetworkError_NonNetworkError(t *testing.T) {
	assert.False(t, isNetworkError(nil))
	assert.False(t, isNetworkError(errors.New("plain error")))
}

// isNetworkError recognizes wrapped syscall errors (ECONNRESET, etc.).
func TestIsNetworkError_SyscallWrapped(t *testing.T) {
	err := &net.OpError{Op: "read", Err: syscall.ECONNRESET}
	assert.True(t, isNetworkError(err))
}

// checkStatusBody builds the JSON served by /api/update/status.
func checkStatusBody(current, latest string, ts int64, outcome string) string {
	return fmt.Sprintf(`{"current":%q,"latest":%q,"available":false,"last_check_ts":%d,"download_url":"","outcome":%q}`,
		current, latest, ts, outcome)
}

// newCheckServer returns an httptest.Server that handles the best-effort
// Check flow (POST kick → single GET status) with the given terminal outcome.
func newCheckServer(t *testing.T, terminalOutcome string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/update/check":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		case "/api/update/status":
			_, _ = w.Write([]byte(checkStatusBody("v1.0.0", "v1.1.0", 200, terminalOutcome)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestUpdateDevice_UpToDate covers the "up_to_date" outcome branch.
func TestUpdateDevice_UpToDate(t *testing.T) {
	server := newCheckServer(t, "up_to_date")
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-device", IP: host, Port: port}

	err := updateDevice(dev)
	assert.NoError(t, err)
}

// TestUpdateDevice_NoAsset covers the "no_asset" outcome branch.
func TestUpdateDevice_NoAsset(t *testing.T) {
	server := newCheckServer(t, "no_asset")
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-device", IP: host, Port: port}

	err := updateDevice(dev)
	assert.NoError(t, err)
}

// TestUpdateDevice_CheckFailed covers the "check_failed" outcome branch.
func TestUpdateDevice_CheckFailed(t *testing.T) {
	server := newCheckServer(t, "check_failed")
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-device", IP: host, Port: port}

	err := updateDevice(dev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update check failed")
}

// TestUpdateDevice_DefaultOutcome covers the default (unknown outcome) branch.
func TestUpdateDevice_DefaultOutcome(t *testing.T) {
	server := newCheckServer(t, "bogus_outcome")
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-device", IP: host, Port: port}

	err := updateDevice(dev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected update check outcome")
}

// TestUpdateDevice_BootMode_CheckUnavailable_ThenTrigger tests the full boot-mode
// path: check routes return 404 (swallowed), trigger returns
// rebooting_for_boot_mode_ota, device reboots and comes back on expected version.
func TestUpdateDevice_BootMode_CheckUnavailable_ThenTrigger(t *testing.T) {
	var applyCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/update/check":
			// Boot-mode device: check route absent.
			w.WriteHeader(http.StatusNotFound)
		case "/api/update/apply":
			applyCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"rebooting_for_boot_mode_ota"}`))
		case "/api/update/progress":
			// Device already rebooting — 404.
			w.WriteHeader(http.StatusNotFound)
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			_, _ = w.Write([]byte(`{"version":"v1.1.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-s2", IP: host, Port: port}

	err := updateDevice(dev)
	require.NoError(t, err)
	assert.Equal(t, int32(1), applyCount.Load(), "apply must be called exactly once")
}

// TestUpdateDevice_BootMode_VersionMismatch tests that a version mismatch after
// boot-mode OTA is reported as a warning but NOT an error.
func TestUpdateDevice_BootMode_VersionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/update/check":
			w.WriteHeader(http.StatusNotFound)
		case "/api/update/apply":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"rebooting_for_boot_mode_ota"}`))
		case "/api/update/progress":
			w.WriteHeader(http.StatusNotFound)
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			// Returns older version than expected.
			_, _ = w.Write([]byte(`{"version":"v1.0.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-s2", IP: host, Port: port}

	// Should succeed (warn only) even when booted version != expected.
	err := updateDevice(dev)
	assert.NoError(t, err)
}

// TestUpdateDevice_BootMode_WithPreCheck tests boot-mode path where the pre-check
// succeeds (returns "available") before the trigger returns boot-mode status.
func TestUpdateDevice_BootMode_WithPreCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/update/check":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		case "/api/update/status":
			_, _ = w.Write([]byte(checkStatusBody("v1.0.0", "v1.1.0", 200, "available")))
		case "/api/update/apply":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"rebooting_for_boot_mode_ota"}`))
		case "/api/update/progress":
			w.WriteHeader(http.StatusNotFound)
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			_, _ = w.Write([]byte(`{"version":"v1.1.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	dev := discover.DeviceInfo{Hostname: "test-s2", IP: host, Port: port}

	err := updateDevice(dev)
	require.NoError(t, err)
}

// TestPollPullUpdate_NetworkErrorWithoutProgress ensures a network error during
// polling does NOT falsely report success when no progress was observed yet.
func TestPollPullUpdate_NetworkErrorWithoutProgress(t *testing.T) {
	// Close a listener immediately so PollStatus hits connection refused on
	// the very first poll (simulates a device that goes away before OTA starts).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	client := ota.NewClient("127.0.0.1", port)
	err = pollPullUpdate(client, "test-device", "v1.1.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "poll failed")
}

// TestPollPullUpdate_NetworkErrorAfterProgress ensures a network error during
// polling IS treated as success when progress was already observed (pull path
// device reboot after flash).
func TestPollPullUpdate_NetworkErrorAfterProgress(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			// First poll: in-progress (sets sawProgress).
			_, _ = w.Write([]byte(`{"state":"updating","in_progress":true,"progress_pct":50}`))
			return
		}
		// Subsequent polls: close connection to simulate reboot.
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	client := ota.NewClient(host, port)

	err := pollPullUpdate(client, "test-device", "v1.1.0")
	// After seeing progress, a network error is success.
	assert.NoError(t, err)
}
