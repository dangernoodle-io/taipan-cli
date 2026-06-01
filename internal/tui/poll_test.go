package tui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
)

func newTestServer(t *testing.T, statsCode int, poolCode int) (*httptest.Server, discover.DeviceInfo) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, _ *http.Request) {
		if statsCode != http.StatusOK {
			w.WriteHeader(statsCode)
			return
		}
		s := device.StatsResponse{
			Hashrate:        500_000,
			HashrateAvg:     490_000,
			TempC:           42.5,
			SessionShares:   10,
			SessionRejected: 0,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	})
	mux.HandleFunc("/api/pool", func(w http.ResponseWriter, _ *http.Request) {
		if poolCode != http.StatusOK {
			w.WriteHeader(poolCode)
			return
		}
		p := device.PoolResponse{
			Host:      "pool.example.com",
			Port:      3333,
			Worker:    "test-worker",
			Connected: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	info := discover.DeviceInfo{
		Hostname: "test-miner",
		IP:       host,
		Port:     port,
	}
	return srv, info
}

func TestPollDevice_Success(t *testing.T) {
	_, info := newTestServer(t, http.StatusOK, http.StatusOK)

	cmd := pollDevice(info)
	require.NotNil(t, cmd)

	msg := cmd()
	polled, ok := msg.(PolledMsg)
	require.True(t, ok)

	assert.Equal(t, "test-miner", polled.Host)
	assert.NoError(t, polled.Err)
	require.NotNil(t, polled.Stats)
	assert.InDelta(t, 500_000.0, polled.Stats.Hashrate, 0.1)
	require.NotNil(t, polled.Pool)
	assert.Equal(t, "pool.example.com", polled.Pool.Host)
}

func TestPollDevice_StatsError(t *testing.T) {
	_, info := newTestServer(t, http.StatusInternalServerError, http.StatusOK)

	msg := pollDevice(info)()
	polled, ok := msg.(PolledMsg)
	require.True(t, ok)

	assert.Equal(t, "test-miner", polled.Host)
	assert.Error(t, polled.Err)
	assert.Nil(t, polled.Stats)
}

func TestPollDevice_PoolError_NonFatal(t *testing.T) {
	_, info := newTestServer(t, http.StatusOK, http.StatusInternalServerError)

	msg := pollDevice(info)()
	polled, ok := msg.(PolledMsg)
	require.True(t, ok)

	assert.NoError(t, polled.Err)
	require.NotNil(t, polled.Stats)
	assert.Nil(t, polled.Pool, "pool error should be non-fatal")
}

func TestPollDevice_ServerClosed(t *testing.T) {
	// Re-create without cleanup to close manually.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // immediate close — connection refused

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	info2 := discover.DeviceInfo{Hostname: "dead-miner", IP: host, Port: port}
	msg := pollDevice(info2)()
	polled, ok := msg.(PolledMsg)
	require.True(t, ok)
	assert.Error(t, polled.Err)
	assert.Nil(t, polled.Stats)
}
