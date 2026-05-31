package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/ui"
)

// newJSONServer creates a test HTTP server that returns the given JSON body for any request.
func newJSONServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
}

// newErrorServer creates a test HTTP server that returns a 500 error for any request.
func newErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
}

// statsFixture is a minimal valid stats JSON response.
const statsFixture = `{"hashrate":1500000000,"hashrate_avg":1400000000,"temp_c":45.5,"session_shares":50,"session_rejected":2,"last_share_ago_s":30,"best_diff":2.5,"uptime_s":3600}`

// infoFixture is a minimal valid info JSON response.
const infoFixture = `{"board":"tdongle-s3","version":"v1.0.0","idf_version":"v5.3.1","cores":2,"mac":"aa:bb:cc:dd:ee:ff","network":{"ssid":"wifi"},"total_heap":327680,"free_heap":200000,"flash_size":4194304,"reset_reason":"power-on","wdt_resets":0}`

// poolFixture is a minimal valid pool JSON response.
const poolFixture = `{"host":"pool.example.com","port":3333,"worker":"test-worker","wallet":"test-wallet","connected":true,"current_difficulty":0.0625,"pool_effective_hashrate":1000000,"lifetime_blocks_total":0,"stats":[]}`

// TestRunStats_JSONOutput_ByteClean verifies --json stdout is valid JSON with no ANSI bytes.
func TestRunStats_JSONOutput_ByteClean(t *testing.T) {
	server := newJSONServer(t, statsFixture)
	defer server.Close()

	h, p := parseTestHostPort(t, server.URL)

	oldJSON, oldAll, oldHosts := statsJSON, statsAll, statsHosts
	defer func() {
		statsJSON = oldJSON
		statsAll = oldAll
		statsHosts = oldHosts
		ui.SetEnabled(true)
	}()

	statsJSON = true
	statsAll = false
	statsHosts = []string{fmt.Sprintf("%s:%d", h, p)}
	ui.SetEnabled(true) // ensure starts enabled; runStats should flip it off

	out := captureStdout(t, func() {
		err := runStats(nil, nil)
		assert.NoError(t, err)
	})

	out = strings.TrimSpace(out)
	var parsed interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "stdout must be valid JSON, got: %q", out)
	assert.NotContains(t, out, "\033[", "stdout must not contain ANSI escape codes")
}

// TestRunInfo_JSONOutput_ByteClean mirrors the stats test for the info command.
func TestRunInfo_JSONOutput_ByteClean(t *testing.T) {
	server := newJSONServer(t, infoFixture)
	defer server.Close()

	h, p := parseTestHostPort(t, server.URL)

	oldJSON, oldAll, oldHosts := infoJSON, infoAll, infoHosts
	defer func() {
		infoJSON = oldJSON
		infoAll = oldAll
		infoHosts = oldHosts
		ui.SetEnabled(true)
	}()

	infoJSON = true
	infoAll = false
	infoHosts = []string{fmt.Sprintf("%s:%d", h, p)}
	ui.SetEnabled(true)

	out := captureStdout(t, func() {
		err := runInfo(nil, nil)
		assert.NoError(t, err)
	})

	out = strings.TrimSpace(out)
	var parsed interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "stdout must be valid JSON, got: %q", out)
	assert.NotContains(t, out, "\033[", "stdout must not contain ANSI escape codes")
}

// TestRunPool_JSONOutput_ByteClean mirrors the stats test for the pool command.
func TestRunPool_JSONOutput_ByteClean(t *testing.T) {
	server := newJSONServer(t, poolFixture)
	defer server.Close()

	h, p := parseTestHostPort(t, server.URL)

	oldJSON, oldAll, oldHosts := poolJSON, poolAll, poolHosts
	defer func() {
		poolJSON = oldJSON
		poolAll = oldAll
		poolHosts = oldHosts
		ui.SetEnabled(true)
	}()

	poolJSON = true
	poolAll = false
	poolHosts = []string{fmt.Sprintf("%s:%d", h, p)}
	ui.SetEnabled(true)

	out := captureStdout(t, func() {
		err := runPool(nil, nil)
		assert.NoError(t, err)
	})

	out = strings.TrimSpace(out)
	var parsed interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "stdout must be valid JSON, got: %q", out)
	assert.NotContains(t, out, "\033[", "stdout must not contain ANSI escape codes")
}

// TestRunStats_ErrorAggregation verifies that per-device errors are counted and non-failing
// devices still produce output.
func TestRunStats_ErrorAggregation(t *testing.T) {
	okServer := newJSONServer(t, statsFixture)
	defer okServer.Close()
	errServer := newErrorServer(t)
	defer errServer.Close()

	okH, okP := parseTestHostPort(t, okServer.URL)
	errH, errP := parseTestHostPort(t, errServer.URL)

	oldJSON, oldAll, oldHosts := statsJSON, statsAll, statsHosts
	defer func() {
		statsJSON = oldJSON
		statsAll = oldAll
		statsHosts = oldHosts
		ui.SetEnabled(true)
	}()

	statsJSON = false
	statsAll = false
	statsHosts = []string{
		fmt.Sprintf("%s:%d", okH, okP),
		fmt.Sprintf("%s:%d", errH, errP),
	}
	ui.SetEnabled(false)

	out := captureStdout(t, func() {
		err := runStats(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1 of 2")
	})

	// The successful device's output must still appear
	assert.Contains(t, out, "Hashrate:")
}

// TestRunStats_MultiDevice_TargetOrder verifies output order matches target slice order,
// not goroutine completion order.
func TestRunStats_MultiDevice_TargetOrder(t *testing.T) {
	// body1 → 1 GH/s, body2 → 2 GH/s so we can distinguish them
	body1 := `{"hashrate":1000000000,"hashrate_avg":1000000000,"temp_c":40.0,"session_shares":10,"session_rejected":0,"last_share_ago_s":5,"best_diff":1.0,"uptime_s":100}`
	body2 := `{"hashrate":2000000000,"hashrate_avg":2000000000,"temp_c":50.0,"session_shares":20,"session_rejected":0,"last_share_ago_s":10,"best_diff":2.0,"uptime_s":200}`

	server1 := newJSONServer(t, body1)
	defer server1.Close()
	server2 := newJSONServer(t, body2)
	defer server2.Close()

	h1, p1 := parseTestHostPort(t, server1.URL)
	h2, p2 := parseTestHostPort(t, server2.URL)

	oldJSON, oldAll, oldHosts := statsJSON, statsAll, statsHosts
	defer func() {
		statsJSON = oldJSON
		statsAll = oldAll
		statsHosts = oldHosts
		ui.SetEnabled(true)
	}()

	// --json so output is structured and easily distinguishable
	statsJSON = true
	statsAll = false
	// Explicit order: h1 first, h2 second
	statsHosts = []string{
		fmt.Sprintf("%s:%d", h1, p1),
		fmt.Sprintf("%s:%d", h2, p2),
	}
	ui.SetEnabled(false)

	out := captureStdout(t, func() {
		err := runStats(nil, nil)
		require.NoError(t, err)
	})

	// 1 GH/s must appear before 2 GH/s
	idx1 := strings.Index(out, `"hashrate": 1000000000`)
	idx2 := strings.Index(out, `"hashrate": 2000000000`)
	assert.True(t, idx1 >= 0, "first device output missing")
	assert.True(t, idx2 >= 0, "second device output missing")
	assert.Less(t, idx1, idx2, "first device must appear before second in output")
}

// TestRunStats_SingleDevice_NoHeader verifies that single-device output has no [hostname] header.
func TestRunStats_SingleDevice_NoHeader(t *testing.T) {
	server := newJSONServer(t, statsFixture)
	defer server.Close()

	h, p := parseTestHostPort(t, server.URL)

	oldJSON, oldAll, oldHosts := statsJSON, statsAll, statsHosts
	defer func() {
		statsJSON = oldJSON
		statsAll = oldAll
		statsHosts = oldHosts
		ui.SetEnabled(true)
	}()

	statsJSON = false
	statsAll = false
	statsHosts = []string{fmt.Sprintf("%s:%d", h, p)}
	ui.SetEnabled(false)

	out := captureStdout(t, func() {
		err := runStats(nil, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "Hashrate:")
	assert.NotContains(t, out, "[127.0.0.1]")
}

// TestRunInfo_MultiDevice_TargetOrder verifies info output preserves target order.
func TestRunInfo_MultiDevice_TargetOrder(t *testing.T) {
	body1 := `{"board":"board-a","version":"v1.0.0","idf_version":"v5.3.1","cores":2,"mac":"aa:bb:cc:dd:ee:ff","network":{"ssid":"wifi"},"total_heap":327680,"free_heap":200000,"flash_size":4194304,"reset_reason":"power-on","wdt_resets":0}`
	body2 := `{"board":"board-b","version":"v2.0.0","idf_version":"v5.3.1","cores":2,"mac":"11:22:33:44:55:66","network":{"ssid":"wifi2"},"total_heap":327680,"free_heap":200000,"flash_size":4194304,"reset_reason":"power-on","wdt_resets":0}`

	server1 := newJSONServer(t, body1)
	defer server1.Close()
	server2 := newJSONServer(t, body2)
	defer server2.Close()

	h1, p1 := parseTestHostPort(t, server1.URL)
	h2, p2 := parseTestHostPort(t, server2.URL)

	oldJSON, oldAll, oldHosts := infoJSON, infoAll, infoHosts
	defer func() {
		infoJSON = oldJSON
		infoAll = oldAll
		infoHosts = oldHosts
		ui.SetEnabled(true)
	}()

	infoJSON = true
	infoAll = false
	infoHosts = []string{
		fmt.Sprintf("%s:%d", h1, p1),
		fmt.Sprintf("%s:%d", h2, p2),
	}
	ui.SetEnabled(false)

	out := captureStdout(t, func() {
		err := runInfo(nil, nil)
		require.NoError(t, err)
	})

	idx1 := strings.Index(out, "board-a")
	idx2 := strings.Index(out, "board-b")
	assert.True(t, idx1 >= 0, "first device output missing")
	assert.True(t, idx2 >= 0, "second device output missing")
	assert.Less(t, idx1, idx2, "first device must appear before second in output")
}

// TestRunPool_MultiDevice_TargetOrder verifies pool output preserves target order.
func TestRunPool_MultiDevice_TargetOrder(t *testing.T) {
	body1 := `{"host":"pool-a.example.com","port":3333,"worker":"worker-a","wallet":"wallet-a","connected":true,"current_difficulty":0.0625,"pool_effective_hashrate":1000000,"lifetime_blocks_total":0,"stats":[]}`
	body2 := `{"host":"pool-b.example.com","port":3334,"worker":"worker-b","wallet":"wallet-b","connected":false,"current_difficulty":0.125,"pool_effective_hashrate":2000000,"lifetime_blocks_total":0,"stats":[]}`

	server1 := newJSONServer(t, body1)
	defer server1.Close()
	server2 := newJSONServer(t, body2)
	defer server2.Close()

	h1, p1 := parseTestHostPort(t, server1.URL)
	h2, p2 := parseTestHostPort(t, server2.URL)

	oldJSON, oldAll, oldHosts := poolJSON, poolAll, poolHosts
	defer func() {
		poolJSON = oldJSON
		poolAll = oldAll
		poolHosts = oldHosts
		ui.SetEnabled(true)
	}()

	poolJSON = true
	poolAll = false
	poolHosts = []string{
		fmt.Sprintf("%s:%d", h1, p1),
		fmt.Sprintf("%s:%d", h2, p2),
	}
	ui.SetEnabled(false)

	out := captureStdout(t, func() {
		err := runPool(nil, nil)
		require.NoError(t, err)
	})

	idx1 := strings.Index(out, "pool-a.example.com")
	idx2 := strings.Index(out, "pool-b.example.com")
	assert.True(t, idx1 >= 0, "first device output missing")
	assert.True(t, idx2 >= 0, "second device output missing")
	assert.Less(t, idx1, idx2, "first device must appear before second in output")
}
