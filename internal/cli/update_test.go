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
		_, _ = w.Write([]byte("v0.7.5"))
	}))
	defer server.Close()

	host, port := parseTestHostPort(t, server.URL)
	client := ota.NewClient(host, port)
	reportBootedVersion(client, "test-host", "v0.7.5")
}

// reportBootedVersion should handle a mismatched booted version gracefully.
func TestReportBootedVersion_Mismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("v0.7.4"))
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

// newCheckServer returns an httptest.Server that sequences through the
// pre-kick GET → POST kick → poll GET pattern used by ota.Client.Check,
// ending at the given terminal outcome.
func newCheckServer(t *testing.T, terminalOutcome string) *httptest.Server {
	t.Helper()
	var reqCount atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick GET /api/update/status
			_, _ = w.Write([]byte(checkStatusBody("v1.0.0", "v1.1.0", 100, "unknown")))
		case 2: // POST /api/update/check
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // poll — ts advanced, terminal outcome
			_, _ = w.Write([]byte(checkStatusBody("v1.0.0", "v1.1.0", 200, terminalOutcome)))
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
