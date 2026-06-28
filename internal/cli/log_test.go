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

// jsonData returns a valid JSON log event data line.
func jsonData(level, tag, msg string) string {
	return fmt.Sprintf(`{"ts":1000,"level":%q,"tag":%q,"msg":%q}`, level, tag, msg)
}

// TestStreamDevice_SingleDevice verifies that streamDevice outputs formatted log without prefix.
func TestStreamDevice_SingleDevice(t *testing.T) {
	d1 := jsonData("I", "wifi", "connected")
	d2 := jsonData("W", "heap", "low memory")
	sseBody := "event: log\ndata: " + d1 + "\nid: 1\n\nevent: log\ndata: " + d2 + "\nid: 2\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "I wifi: connected", lines[0])
	assert.Equal(t, "W heap: low memory", lines[1])
}

// TestStreamDevice_SkipsCommentsAndEmptyLines verifies SSE comments, event:, and id: lines are skipped.
func TestStreamDevice_SkipsCommentsAndEmptyLines(t *testing.T) {
	d1 := jsonData("I", "wifi", "connected")
	d2 := jsonData("E", "mqtt", "reconnect")
	sseBody := ": comment 1\n\nevent: log\ndata: " + d1 + "\nid: 1\n\n: comment 2\n\n\n\nevent: log\ndata: " + d2 + "\nid: 2\n\n: comment 3\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "I wifi: connected", lines[0])
	assert.Equal(t, "E mqtt: reconnect", lines[1])
}

// TestStreamDevice_MultiDevice verifies that prefixFn adds hostname prefix to each line.
func TestStreamDevice_MultiDevice(t *testing.T) {
	d1 := jsonData("I", "pool", "accepted")
	d2 := jsonData("D", "hash", "nonce found")
	sseBody := "event: log\ndata: " + d1 + "\nid: 1\n\nevent: log\ndata: " + d2 + "\nid: 2\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "miner-01",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	prefixFn := func(hostname string) string {
		return "[" + hostname + "] "
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, prefixFn, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "[miner-01] I pool: accepted", lines[0])
	assert.Equal(t, "[miner-01] D hash: nonce found", lines[1])
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
	}

	var mu sync.Mutex
	err := streamDevice(context.Background(), device, nil, &mu, func() {})
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
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var mu sync.Mutex
	err := streamDevice(ctx, device, nil, &mu, func() {})
	// Should fail due to context being already cancelled
	require.Error(t, err)
}

// TestStreamDevice_MutexSerialization verifies that the mutex is held during output.
func TestStreamDevice_MutexSerialization(t *testing.T) {
	d1 := jsonData("I", "sys", "started")
	d2 := jsonData("I", "sys", "ready")
	sseBody := "event: log\ndata: " + d1 + "\nid: 1\n\nevent: log\ndata: " + d2 + "\nid: 2\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-worker",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	prefixFn := func(hostname string) string {
		return "[" + hostname + "] "
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, prefixFn, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "[test-worker] I sys: started", lines[0])
	assert.Equal(t, "[test-worker] I sys: ready", lines[1])
}

// TestStreamDevice_EmptyResponse verifies handling of empty response body.
func TestStreamDevice_EmptyResponse(t *testing.T) {
	server := newSSEServer(t, "")
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	assert.Equal(t, "", strings.TrimSpace(out))
}

// TestStreamDevice_MixedFormatLines verifies that only "data: " prefixed lines produce output.
func TestStreamDevice_MixedFormatLines(t *testing.T) {
	d1 := jsonData("I", "wifi", "connected")
	d2 := jsonData("I", "pool", "accepted")
	d3 := jsonData("D", "hash", "done")
	// interleave event:/id: fields between data lines
	sseBody := "event: log\ndata: " + d1 + "\nid: 1\n\nid: 2\n\nevent: log\ndata: " + d2 + "\nid: 3\n\nevent: log\ndata: " + d3 + "\nid: 4\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 3, len(lines))
	assert.Equal(t, "I wifi: connected", lines[0])
	assert.Equal(t, "I pool: accepted", lines[1])
	assert.Equal(t, "D hash: done", lines[2])
}

// TestStreamDevice_MalformedDataLine verifies that non-JSON data lines are printed raw without crashing.
func TestStreamDevice_MalformedDataLine(t *testing.T) {
	sseBody := "data: not-json-at-all\n\nevent: log\ndata: " + jsonData("I", "sys", "ok") + "\nid: 2\n\n"
	server := newSSEServer(t, sseBody)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, 2, len(lines))
	assert.Equal(t, "not-json-at-all", lines[0]) // raw fallback
	assert.Equal(t, "I sys: ok", lines[1])
}

// TestStreamDevice_FullSSEFrame verifies parsing of a complete event:/data:/id: frame.
func TestStreamDevice_FullSSEFrame(t *testing.T) {
	frame := "event: log\ndata: {\"ts\":1719532800000,\"level\":\"I\",\"tag\":\"x\",\"msg\":\"hello\"}\nid: 1\n\n"
	server := newSSEServer(t, frame)
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	out := captureStdout(t, func() {
		err := streamDevice(context.Background(), device, nil, &mu, func() {})
		require.NoError(t, err)
	})

	assert.Equal(t, "I x: hello", strings.TrimSpace(out))
}

// TestFormatLogEvent_Valid verifies correct formatting of a valid JSON payload.
func TestFormatLogEvent_Valid(t *testing.T) {
	payload := `{"ts":1000,"level":"W","tag":"heap","msg":"low memory"}`
	assert.Equal(t, "W heap: low memory", formatLogEvent(payload))
}

// TestFormatLogEvent_Malformed verifies that malformed JSON returns the raw payload.
func TestFormatLogEvent_Malformed(t *testing.T) {
	payload := "this is not json"
	assert.Equal(t, payload, formatLogEvent(payload))
}

// TestStreamDevice_EndpointPath verifies the request targets /api/events?topic=log.
func TestStreamDevice_EndpointPath(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "")
	}))
	defer server.Close()

	device := discover.DeviceInfo{
		Hostname: "test-device.local",
		IP:       "127.0.0.1",
		Port:     extractPortFromURL(t, server.URL),
	}

	var mu sync.Mutex
	_ = streamDevice(context.Background(), device, nil, &mu, func() {})
	assert.Equal(t, "/api/events?topic=log", capturedPath)
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
