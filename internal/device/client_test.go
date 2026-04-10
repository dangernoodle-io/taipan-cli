package device

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

// TestStats_OK tests Stats with a valid response.
func TestStats_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/stats", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := StatsResponse{
			Hashrate:        1.5e9,
			HashrateAvg:     1.4e9,
			TempC:           45.5,
			Shares:          100,
			PoolDifficulty:  0.0625,
			SessionShares:   50,
			SessionRejected: 2,
			LastShareAgoS:   30,
			LifetimeShares:  10000,
			BestDiff:        2.5,
			PoolHost:        "pool.example.com",
			PoolPort:        3333,
			Worker:          "test-worker",
			Wallet:          "test-wallet-addr",
			UptimeS:         3600,
			Version:         "v1.2.3",
			BuildDate:       "2026-04-09",
			BuildTime:       "12:34:56",
			Board:           "tdongle-s3",
			DisplayEn:       true,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	stats, err := client.Stats(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1.5e9, stats.Hashrate)
	assert.Equal(t, 1.4e9, stats.HashrateAvg)
	assert.Equal(t, 45.5, stats.TempC)
	assert.Equal(t, uint32(100), stats.Shares)
	assert.Equal(t, 0.0625, stats.PoolDifficulty)
	assert.Equal(t, uint32(50), stats.SessionShares)
	assert.Equal(t, uint32(2), stats.SessionRejected)
	assert.Equal(t, int64(30), stats.LastShareAgoS)
	assert.Equal(t, float64(10000), stats.LifetimeShares)
	assert.Equal(t, 2.5, stats.BestDiff)
	assert.Equal(t, "pool.example.com", stats.PoolHost)
	assert.Equal(t, 3333, stats.PoolPort)
	assert.Equal(t, "test-worker", stats.Worker)
	assert.Equal(t, "test-wallet-addr", stats.Wallet)
	assert.Equal(t, 3600.0, stats.UptimeS)
	assert.Equal(t, "v1.2.3", stats.Version)
	assert.Equal(t, "tdongle-s3", stats.Board)
	assert.True(t, stats.DisplayEn)
	assert.Nil(t, stats.AsicHashrate)
}

// TestStats_ASICBoard tests Stats with ASIC board fields.
func TestStats_ASICBoard(t *testing.T) {
	asicHashrate := 5.0e9
	asicHashrateAvg := 4.9e9
	asicTempC := 65.0
	asicShares := uint32(45)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/stats", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := StatsResponse{
			Hashrate:        5.0e9,
			HashrateAvg:     4.9e9,
			TempC:           45.5,
			Shares:          100,
			PoolDifficulty:  0.0625,
			SessionShares:   50,
			SessionRejected: 2,
			LastShareAgoS:   30,
			LifetimeShares:  10000,
			BestDiff:        2.5,
			PoolHost:        "pool.example.com",
			PoolPort:        3333,
			Worker:          "test-worker",
			Wallet:          "test-wallet-addr",
			UptimeS:         3600,
			Version:         "v1.2.3",
			BuildDate:       "2026-04-09",
			BuildTime:       "12:34:56",
			Board:           "tdongle-asic",
			DisplayEn:       true,
			AsicHashrate:    &asicHashrate,
			AsicHashrateAvg: &asicHashrateAvg,
			AsicTempC:       &asicTempC,
			AsicShares:      &asicShares,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	stats, err := client.Stats(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, stats.AsicHashrate)
	assert.Equal(t, 5.0e9, *stats.AsicHashrate)
	assert.NotNil(t, stats.AsicHashrateAvg)
	assert.Equal(t, 4.9e9, *stats.AsicHashrateAvg)
	assert.NotNil(t, stats.AsicTempC)
	assert.Equal(t, 65.0, *stats.AsicTempC)
	assert.NotNil(t, stats.AsicShares)
	assert.Equal(t, uint32(45), *stats.AsicShares)
}

// TestStats_HTTPError tests Stats with a server error response.
func TestStats_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	stats, err := client.Stats(context.Background())

	assert.Nil(t, stats)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestStats_BadJSON tests Stats with invalid JSON response.
func TestStats_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	stats, err := client.Stats(context.Background())

	assert.Nil(t, stats)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

// TestInfo_OK tests Info with a valid response.
func TestInfo_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/info", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := InfoResponse{
			Board:       "tdongle-s3",
			ProjectName: "TaipanMiner",
			Version:     "v1.2.3",
			IDFVersion:  "v5.3.1",
			BuildDate:   "2026-04-09",
			BuildTime:   "12:34:56",
			Cores:       2,
			MAC:         "aa:bb:cc:dd:ee:ff",
			WorkerName:  "rig01",
			SSID:        "HomeWifi",
			TotalHeap:   327680,
			FreeHeap:    200000,
			FlashSize:   4194304,
			ResetReason: "power-on",
			WDTResets:   0,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	info, err := client.Info(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "tdongle-s3", info.Board)
	assert.Equal(t, "TaipanMiner", info.ProjectName)
	assert.Equal(t, "v1.2.3", info.Version)
	assert.Equal(t, "v5.3.1", info.IDFVersion)
	assert.Equal(t, 2, info.Cores)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", info.MAC)
	assert.Equal(t, "rig01", info.WorkerName)
	assert.Equal(t, "HomeWifi", info.SSID)
	assert.Equal(t, uint64(327680), info.TotalHeap)
	assert.Equal(t, uint64(200000), info.FreeHeap)
	assert.Equal(t, uint32(4194304), info.FlashSize)
	assert.Equal(t, "power-on", info.ResetReason)
	assert.Equal(t, 0, info.WDTResets)
}

// TestInfo_HTTPError tests Info with a server error response.
func TestInfo_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	info, err := client.Info(context.Background())

	assert.Nil(t, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestGetSettings_OK tests GetSettings with a valid response.
func TestGetSettings_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/settings", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := SettingsResponse{
			PoolHost:     "pool.example.com",
			PoolPort:     3333,
			Wallet:       "wallet-addr",
			Worker:       "test-worker",
			PoolPass:     "password123",
			DisplayEn:    true,
			OTASkipCheck: false,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	settings, err := client.GetSettings(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "pool.example.com", settings.PoolHost)
	assert.Equal(t, 3333, settings.PoolPort)
	assert.Equal(t, "wallet-addr", settings.Wallet)
	assert.Equal(t, "test-worker", settings.Worker)
	assert.Equal(t, "password123", settings.PoolPass)
	assert.True(t, settings.DisplayEn)
	assert.False(t, settings.OTASkipCheck)
}

// TestGetSettings_HTTPError tests GetSettings with a server error response.
func TestGetSettings_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	settings, err := client.GetSettings(context.Background())

	assert.Nil(t, settings)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestSetSetting_OK tests SetSetting with a valid response.
func TestSetSetting_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/settings", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		defer func() { _ = r.Body.Close() }()

		var payload map[string]any
		err = json.Unmarshal(body, &payload)
		require.NoError(t, err)
		assert.Contains(t, payload, "pool_host")
		assert.Equal(t, "newpool.example.com", payload["pool_host"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := SetSettingResponse{
			Status:         "ok",
			RebootRequired: false,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.SetSetting(context.Background(), "pool_host", "newpool.example.com")

	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.False(t, response.RebootRequired)
}

// TestSetSetting_RebootNotRequired tests SetSetting with reboot_required: false.
func TestSetSetting_RebootNotRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/settings", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := SetSettingResponse{
			Status:         "ok",
			RebootRequired: false,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.SetSetting(context.Background(), "display_en", true)

	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.False(t, response.RebootRequired)
}

// TestSetSetting_RebootRequired tests SetSetting with reboot_required: true.
func TestSetSetting_RebootRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/settings", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := SetSettingResponse{
			Status:         "ok",
			RebootRequired: true,
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.SetSetting(context.Background(), "pool_host", "newpool.example.com")

	require.NoError(t, err)
	assert.Equal(t, "ok", response.Status)
	assert.True(t, response.RebootRequired)
}

// TestSetSetting_HTTPError tests SetSetting with a server error response.
func TestSetSetting_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.SetSetting(context.Background(), "pool_host", "newpool.example.com")

	assert.Nil(t, response)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

// TestReboot_OK tests Reboot with a valid response.
func TestReboot_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/reboot", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		result := RebootResponse{
			Status: "rebooting",
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.Reboot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "rebooting", response.Status)
}

// TestReboot_HTTPError tests Reboot with a server error response.
func TestReboot_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	host, port, err := parseTestServerURL(server.URL)
	require.NoError(t, err)

	client := NewClient(host, port)
	response, err := client.Reboot(context.Background())

	assert.Nil(t, response)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}
