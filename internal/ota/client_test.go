package ota

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

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

// TestCheck_Available tests Check with an available update.
func TestCheck_Available(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/ota/check", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := CheckResult{
			CurrentVersion:  "v1.0.0",
			LatestVersion:   "v1.1.0",
			UpdateAvailable: true,
			Asset:           "firmware.bin",
		}
		_ = json.NewEncoder(w).Encode(result)
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
	assert.Equal(t, "firmware.bin", result.Asset)
}

// TestCheck_NotAvailable tests Check with no update available.
func TestCheck_NotAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/ota/check", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := CheckResult{
			CurrentVersion:  "v1.1.0",
			LatestVersion:   "v1.1.0",
			UpdateAvailable: false,
			Asset:           "",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	result, err := client.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", result.CurrentVersion)
	assert.Equal(t, "v1.1.0", result.LatestVersion)
	assert.False(t, result.UpdateAvailable)
}

// TestCheck_HTTPError tests Check with a server error response.
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

// TestCheck_BadJSON tests Check with invalid JSON response.
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

// TestCheck_PollOn202 tests Check polling on 202 Accepted responses.
func TestCheck_PollOn202(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/ota/check", r.URL.Path)

		count := requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")

		switch count {
		case 1:
			// First request returns 202 Accepted
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("{}"))
		case 2:
			// Second request returns 200 OK with CheckResult
			w.WriteHeader(http.StatusOK)
			result := CheckResult{
				CurrentVersion:  "v1.0.0",
				LatestVersion:   "v1.1.0",
				UpdateAvailable: true,
				Asset:           "firmware.bin",
			}
			_ = json.NewEncoder(w).Encode(result)
		default:
			t.Fatal("unexpected extra request")
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
	assert.Equal(t, "firmware.bin", result.Asset)
	assert.Equal(t, int32(2), requestCount.Load())
}

// TestTrigger_Started tests Trigger with a successful update start (202).
func TestTrigger_Started(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/ota/update", r.URL.Path)

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
		require.Equal(t, "/api/ota/update", r.URL.Path)

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
		require.Equal(t, "/api/ota/update", r.URL.Path)

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
		require.Equal(t, "/api/ota/status", r.URL.Path)

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
		require.Equal(t, "/api/ota/status", r.URL.Path)

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
		require.Equal(t, "/api/ota/status", r.URL.Path)

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
