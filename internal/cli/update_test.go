package cli

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
