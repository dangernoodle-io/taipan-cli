package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTestServerURL extracts host and port from a test server URL.
func parseTestServerURL(serverURL string) (string, int, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", 0, err
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// statusBody builds a JSON body for /api/update/status with outcome enum.
func statusBody(current, latest string, available bool, ts int64, outcome, downloadURL string) string {
	return fmt.Sprintf(`{"current":%q,"latest":%q,"available":%v,"last_check_ts":%d,"download_url":%q,"outcome":%q}`,
		current, latest, available, ts, downloadURL, outcome)
}

// TestCheck_Available tests Check with outcome="available".
func TestCheck_Available(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick GET /api/update/status
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/api/update/status", r.URL.Path)
			_, _ = w.Write([]byte(statusBody("v1.0.0", "v1.1.0", true, 100, "unknown", "")))
		case 2: // POST /api/update/check
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/update/check", r.URL.Path)
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // poll — ts advanced, outcome terminal
			require.Equal(t, "/api/update/status", r.URL.Path)
			_, _ = w.Write([]byte(statusBody("v1.0.0", "v1.1.0", true, 200, "available", "https://example.com/fw.bin")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", result.CurrentVersion)
	assert.Equal(t, "v1.1.0", result.LatestVersion)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "available", result.Outcome)
	assert.Equal(t, "https://example.com/fw.bin", result.DownloadURL)
}

// TestCheck_UpToDate tests Check with outcome="up_to_date".
func TestCheck_UpToDate(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick
			_, _ = w.Write([]byte(statusBody("v1.1.0", "v1.1.0", false, 100, "unknown", "")))
		case 2: // kick
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // poll — ts advanced, outcome up_to_date
			_, _ = w.Write([]byte(statusBody("v1.1.0", "v1.1.0", false, 200, "up_to_date", "")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", result.CurrentVersion)
	assert.False(t, result.UpdateAvailable)
	assert.Equal(t, "up_to_date", result.Outcome)
}

// TestCheck_NoAsset tests that outcome="no_asset" returns quickly without hanging.
func TestCheck_NoAsset(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick
			_, _ = w.Write([]byte(statusBody("v1.0.0", "", false, 100, "unknown", "")))
		case 2: // kick
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // poll — ts advanced, outcome no_asset
			_, _ = w.Write([]byte(statusBody("v1.0.0", "", false, 200, "no_asset", "")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)

	start := time.Now()
	result, err := client.Check(context.Background())
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "no_asset", result.Outcome)
	// Must return quickly — not hang on the 2s poll interval more than once.
	assert.Less(t, elapsed, 5*time.Second)
}

// TestCheck_CheckFailed tests that outcome="check_failed" is returned without looping.
func TestCheck_CheckFailed(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick
			_, _ = w.Write([]byte(statusBody("v1.0.0", "", false, 100, "unknown", "")))
		case 2: // kick
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // poll — ts advanced, outcome check_failed
			_, _ = w.Write([]byte(statusBody("v1.0.0", "", false, 200, "check_failed", "")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "check_failed", result.Outcome)
}

// TestCheck_PendingThenTerminal tests that outcome="unknown" keeps polling until terminal.
func TestCheck_PendingThenTerminal(t *testing.T) {
	var reqCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1: // pre-kick — ts=100
			_, _ = w.Write([]byte(statusBody("v1.0.0", "v1.1.0", true, 100, "unknown", "")))
		case 2: // kick
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		case 3: // first poll — ts unchanged, still unknown
			_, _ = w.Write([]byte(statusBody("v1.0.0", "v1.1.0", true, 100, "unknown", "")))
		default: // second poll — ts advanced, terminal outcome
			_, _ = w.Write([]byte(statusBody("v1.0.0", "v1.1.0", true, 200, "available", "https://example.com/fw.bin")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "available", result.Outcome)
	assert.GreaterOrEqual(t, reqCount.Load(), int32(4))
}

// TestCheck_503NotInitialized tests that 503 returns "not initialized" error.
func TestCheck_503NotInitialized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"not initialized"}`))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// TestCheck_BackstopDeadline tests that a context deadline fires instead of hanging
// when the server always returns outcome="unknown".
func TestCheck_BackstopDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost: // kick
			_, _ = w.Write([]byte(`{"status":"checking"}`))
		default: // always return ts=100, outcome=unknown
			_, _ = w.Write([]byte(statusBody("v1.0.0", "", false, 100, "unknown", "")))
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := client.Check(ctx)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestCheck_HTTPError tests Check when the pre-kick status call fails.
func TestCheck_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestCheck_BadJSON tests Check when /api/update/status returns invalid JSON.
func TestCheck_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

// TestTrigger_Started tests Trigger with a successful update start (202).
func TestTrigger_Started(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/update/apply", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		result := TriggerResult{
			Status: "update_started",
			Error:  "",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, statusCode, err := client.Trigger(context.Background())

	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, statusCode)
	assert.Equal(t, "update_started", result.Status)
	assert.Empty(t, result.Error)
}

// TestTrigger_AlreadyUpToDate tests Trigger when already up to date (200).
func TestTrigger_AlreadyUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/update/apply", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := TriggerResult{
			Status: "already_up_to_date",
			Error:  "",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, statusCode, err := client.Trigger(context.Background())

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "already_up_to_date", result.Status)
}

// TestTrigger_Conflict tests Trigger with an in-progress update (409).
func TestTrigger_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/update/apply", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		result := TriggerResult{
			Status: "error",
			Error:  "update_in_progress",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, statusCode, err := client.Trigger(context.Background())

	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, statusCode)
	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "update_in_progress", result.Error)
}

// TestPollStatus_InProgress tests PollStatus with an in-progress update.
func TestPollStatus_InProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/update/progress", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := StatusResult{
			State:       "updating",
			InProgress:  true,
			ProgressPct: 50.0,
			LastError:   "",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.PollStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "updating", result.State)
	assert.True(t, result.InProgress)
	assert.Equal(t, 50.0, result.ProgressPct)
	assert.Empty(t, result.LastError)
}

// TestPollStatus_Complete tests PollStatus with a completed update.
func TestPollStatus_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/update/progress", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := StatusResult{
			State:       "idle",
			InProgress:  false,
			ProgressPct: 100.0,
			LastError:   "",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.PollStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "idle", result.State)
	assert.False(t, result.InProgress)
	assert.Equal(t, 100.0, result.ProgressPct)
}

// TestPollStatus_Error tests PollStatus with an error state.
func TestPollStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/update/progress", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := StatusResult{
			State:       "error",
			InProgress:  false,
			ProgressPct: 0.0,
			LastError:   "flash failed",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.PollStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "error", result.State)
	assert.False(t, result.InProgress)
	assert.Equal(t, "flash failed", result.LastError)
}

// TestPollStatus_HTTPError tests PollStatus with a server error response.
func TestPollStatus_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.PollStatus(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestCheck_ContextCancellation tests Check with a cancelled context.
func TestCheck_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response that will be cancelled
		<-r.Context().Done()
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := client.Check(ctx)

	assert.Nil(t, result)
	assert.Error(t, err)
}

// TestTrigger_ContextCancellation tests Trigger with a cancelled context.
func TestTrigger_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, statusCode, err := client.Trigger(ctx)

	assert.Nil(t, result)
	assert.Zero(t, statusCode)
	assert.Error(t, err)
}

// TestPollStatus_ContextCancellation tests PollStatus with a cancelled context.
func TestPollStatus_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := client.PollStatus(ctx)

	assert.Nil(t, result)
	assert.Error(t, err)
}

// TestFetchVersion_Success tests FetchVersion happy path — reads /api/info JSON.
func TestFetchVersion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/info", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"v0.7.5","board":"tdongle-s3"}`))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	v, err := client.FetchVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v0.7.5", v)
}

// TestFetchVersion_Non200 returns error.
func TestFetchVersion_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	v, err := client.FetchVersion(context.Background())
	assert.Error(t, err)
	assert.Empty(t, v)
}

// TestWaitForBoot_ReadyImmediately returns without waiting a full interval.
func TestWaitForBoot_ReadyImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/info":
			_, _ = w.Write([]byte(`{"version":"v0.7.5","board":"tdongle-s3"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	v, err := client.WaitForBoot(ctx, 500*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, "v0.7.5", v)
	assert.Less(t, time.Since(start), 1500*time.Millisecond)
}

// TestWaitForBoot_DeadlineExceeded returns an error when device never responds.
func TestWaitForBoot_DeadlineExceeded(t *testing.T) {
	// Use a closed listener so every request fails fast.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	client := NewClient("127.0.0.1", port)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	v, err := client.WaitForBoot(ctx, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Empty(t, v)
}
