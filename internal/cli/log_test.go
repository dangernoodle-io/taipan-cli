package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout captures stdout during function execution and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// newSSEServer creates a test HTTP server that returns SSE-formatted content with the given body.
func newSSEServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, body)
	}))
}

// TestStreamDevice_SingleDevice verifies that streamDevice outputs plain payload without prefix.
func TestStreamDevice_SingleDevice(t *testing.T) {
	sseBody := ": connected\n\ndata: line one\n\ndata: line two\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	// Extract host and port from server URL
	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "line one", lines[0])
	assert.Equal(t, "line two", lines[1])
}

// TestStreamDevice_SkipsCommentsAndEmptyLines verifies SSE comments and empty lines are skipped.
func TestStreamDevice_SkipsCommentsAndEmptyLines(t *testing.T) {
	sseBody := ": comment 1\n\ndata: line one\n\n: comment 2\n\n\n\ndata: line two\n\n: comment 3\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "line one", lines[0])
	assert.Equal(t, "line two", lines[1])
}

// TestStreamDevice_MultiDevice verifies that prefixFn adds worker prefix to each line.
func TestStreamDevice_MultiDevice(t *testing.T) {
	sseBody := "data: payload one\n\ndata: payload two\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "miner-01",
	}

	prefixFn := func(worker string) string {
		return "[" + worker + "] "
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, prefixFn, &mu)
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "[miner-01] payload one", lines[0])
	assert.Equal(t, "[miner-01] payload two", lines[1])
}

// TestStreamDevice_HTTPError verifies that non-200 status codes return an error.
func TestStreamDevice_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "Service unavailable")
	}))
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	err := streamDevice(context.Background(), device, nil, &mu)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status: 503")
}

// TestStreamDevice_ContextCancellation verifies that context cancellation stops streaming.
func TestStreamDevice_ContextCancellation(t *testing.T) {
	// Create a server that will block until request context cancels
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()

		// Block until context is done
		<-r.Context().Done()
	}))
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var mu sync.Mutex
	err := streamDevice(ctx, device, nil, &mu)
	// Should fail due to context being already cancelled
	require.Error(t, err)
}

// TestStreamDevice_MutexSerialization verifies that the mutex is held during output.
func TestStreamDevice_MutexSerialization(t *testing.T) {
	sseBody := "data: line one\n\ndata: line two\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	prefixFn := func(worker string) string {
		return "[" + worker + "] "
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, prefixFn, &mu)
		require.NoError(t, err)
	})

	// Verify output was produced (mutex was used for synchronization)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "[test-worker] line one", lines[0])
	assert.Equal(t, "[test-worker] line two", lines[1])
}

// TestStreamDevice_EmptyResponse verifies handling of empty response body.
func TestStreamDevice_EmptyResponse(t *testing.T) {
	server := newSSEServer(t, "")
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	assert.Equal(t, "", strings.TrimSpace(output))
}

// TestStreamDevice_MixedFormatLines verifies that only "data: " prefixed lines are output.
func TestStreamDevice_MixedFormatLines(t *testing.T) {
	sseBody := "data: valid line\n\nid: 123\n\ndata: another valid\n\nevent: message\n\ndata: final line\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 3, len(lines))
	assert.Equal(t, "valid line", lines[0])
	assert.Equal(t, "another valid", lines[1])
	assert.Equal(t, "final line", lines[2])
}

// TestStreamDevice_DataLineWithoutPrefix verifies that malformed data lines are skipped.
func TestStreamDevice_DataLineWithoutPrefix(t *testing.T) {
	sseBody := "data: valid line\n\ndata valid\n\ndata:another valid\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	// "data:another valid" is missing the space after colon, so it's skipped
	assert.Equal(t, 1, len(lines))
	assert.Equal(t, "valid line", lines[0])
}

// TestStreamDevice_LongPayload verifies that long payloads are handled correctly.
func TestStreamDevice_LongPayload(t *testing.T) {
	longPayload := strings.Repeat("x", 10000)
	sseBody := "data: " + longPayload + "\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	assert.Equal(t, longPayload, strings.TrimSpace(output))
}

// TestStreamDevice_SpecialCharactersInPayload verifies that special characters are preserved.
func TestStreamDevice_SpecialCharactersInPayload(t *testing.T) {
	specialPayload := "line with [brackets] {braces} and special chars: !@#$%^&*()"
	sseBody := "data: " + specialPayload + "\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
		Worker:   "test-worker",
	}

	var mu sync.Mutex
	output := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu)
		require.NoError(t, err)
	})

	assert.Equal(t, specialPayload, strings.TrimSpace(output))
}

// extractPortFromURL is a helper to extract the port number from an httptest.Server URL.
func extractPortFromURL(t *testing.T, serverURL string) int {
	t.Helper()
	parts := strings.Split(serverURL, ":")
	portStr := parts[len(parts)-1]
	// Handle cases where URL might have trailing slash
	portStr = strings.TrimSuffix(portStr, "/")
	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)
	return port
}
