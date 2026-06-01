package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
)

// ── fmtDuration ──────────────────────────────────────────────────────────────

func TestFmtDuration_Hours(t *testing.T) {
	out := fmtDuration(3723) // 1h 2m 3s
	assert.Contains(t, out, "h")
	assert.Contains(t, out, "m")
	assert.Contains(t, out, "s")
}

func TestFmtDuration_Minutes(t *testing.T) {
	out := fmtDuration(125) // 2m 5s
	assert.NotContains(t, out, "h")
	assert.Contains(t, out, "m")
	assert.Contains(t, out, "s")
}

func TestFmtDuration_Seconds(t *testing.T) {
	out := fmtDuration(45)
	assert.NotContains(t, out, "h")
	assert.NotContains(t, out, "m ")
	assert.Contains(t, out, "s")
}

// ── fmtDiff ───────────────────────────────────────────────────────────────────

func TestFmtDiff_AllBranches(t *testing.T) {
	cases := []struct {
		input    float64
		contains string
	}{
		{2e12, "T"},
		{5e9, "G"},
		{3e6, "M"},
		{500e3, "K"},
		{42.0, "42.00"},
	}
	for _, tc := range cases {
		out := fmtDiff(tc.input)
		assert.Contains(t, out, tc.contains, "fmtDiff(%g)", tc.input)
	}
}

// ── fmtBytes ──────────────────────────────────────────────────────────────────

func TestFmtBytes_AllBranches(t *testing.T) {
	assert.Contains(t, fmtBytes(2<<20), "MB")
	assert.Contains(t, fmtBytes(2<<10), "KB")
	assert.Contains(t, fmtBytes(512), "B")
	assert.NotContains(t, fmtBytes(512), "KB")
}

// ── renderDetail branches ────────────────────────────────────────────────────

func TestRenderDetail_Empty_NoTargets(t *testing.T) {
	m := NewModel(stubDiscover(nil, nil))
	out := renderDetail(m)
	assert.Empty(t, out)
}

func TestRenderDetail_SelectionOutOfBounds(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	m.selected = 99
	out := renderDetail(m)
	assert.Empty(t, out)
}

func TestRenderDetail_NoState(t *testing.T) {
	// target present but no state (discovering phase essentially)
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)
	// don't send any PolledMsg — state is empty
	m.selected = 0

	out := renderDetail(m)
	assert.Contains(t, out, "miner-a")
	assert.Contains(t, out, "offline")
}

func TestRenderDetail_PoolNil(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50},
		Pool: nil,
	})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50}})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	assert.Contains(t, out, "no pool data")
}

func TestRenderDetail_InfoNil(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50},
		Pool:  &device.PoolResponse{Host: "pool.example.com", Port: 3333, Connected: true},
		Info:  nil,
	})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50}})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	assert.Contains(t, out, "no device info")
}

func TestRenderDetail_AsicFields(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	asicHR := 2_000_000_000_000.0
	asicTemp := 68.0
	var asicShares uint32 = 5000
	stats := &device.StatsResponse{
		Hashrate:        1e9,
		TempC:           50,
		AsicHashrate:    &asicHR,
		AsicHashrateAvg: &asicHR,
		AsicTempC:       &asicTemp,
		AsicShares:      &asicShares,
	}
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: stats,
		Pool:  &device.PoolResponse{Host: "pool.example.com", Port: 3333},
	})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50}})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	assert.Contains(t, out, "ASIC")
	assert.Contains(t, out, "2.00 TH/s")
}

func TestRenderDetail_PoolWithOptionalFields(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	latency := 42
	sessionAge := int64(3600)
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50},
		Pool: &device.PoolResponse{
			Host:             "pool.example.com",
			Port:             3333,
			Worker:           "test-worker",
			Connected:        true,
			LatencyMs:        &latency,
			SessionStartAgoS: &sessionAge,
		},
	})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50}})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	assert.Contains(t, out, "42 ms")
	// session age 3600s = 1h
	assert.Contains(t, out, "h")
}

func TestRenderDetail_InfoBoard_FallbackToDiscovery(t *testing.T) {
	// info.Board == "" — falls back to d.Board from discovery
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50},
		Info: &device.InfoResponse{
			Board:   "", // empty → fall back
			Version: "", // empty → fall back
			Network: device.InfoNetwork{SSID: "my-wifi"},
		},
	})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e9, TempC: 50}})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	// d.Board = "esp32s3" from makeTargets
	assert.Contains(t, out, "esp32s3")
}

func TestRenderDetailFooter(t *testing.T) {
	out := renderDetailFooter()
	assert.Contains(t, out, "esc back")
	assert.Contains(t, out, "r refresh")
	assert.Contains(t, out, "q quit")
}

func TestModel_DetailView_WithViewport(t *testing.T) {
	// trigger viewport path by going through WindowSizeMsg in detail mode
	m := setupDetailReady(t)

	// switch to detail via Update (sets viewport size)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.mode = modeDetail
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	assert.NotPanics(t, func() { _ = m.View() })
}

func TestRenderDetail_OfflineWithError(t *testing.T) {
	m := NewModel(stubDiscover([]discover.DeviceInfo{{Hostname: "miner-x", Board: "esp32", Version: "v1.0"}}, nil))
	updated, _ := m.Update(discoveredMsg{targets: []discover.DeviceInfo{{Hostname: "miner-x", Board: "esp32", Version: "v1.0"}}})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{Host: "miner-x", Err: errTest("connection refused")})
	m = updated.(Model)

	m.selected = 0
	out := renderDetail(m)
	assert.Contains(t, out, "miner-x")
	assert.Contains(t, out, "offline")
	assert.Contains(t, out, "connection refused")
}

// errTest is a simple error for testing.
type errTest string

func (e errTest) Error() string { return string(e) }
