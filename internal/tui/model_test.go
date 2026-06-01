package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
)

func makeTargets() []discover.DeviceInfo {
	return []discover.DeviceInfo{
		{Hostname: "miner-a", Board: "esp32s3", Version: "v1.2.3", IP: "192.168.1.10", Port: 80},
		{Hostname: "miner-b", Board: "esp32s3", Version: "v1.2.3", IP: "192.168.1.11", Port: 80},
	}
}

func stubDiscover(targets []discover.DeviceInfo, err error) func() ([]discover.DeviceInfo, error) {
	return func() ([]discover.DeviceInfo, error) { return targets, err }
}

// helper: feed discovery + two polled msgs with different pools.
func setupTwoMiners(t *testing.T) Model {
	t.Helper()
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	hr1 := 310_000_000_000.0 // 0.31 TH/s
	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{
			Hashrate:        hr1,
			TempC:           52.4,
			SessionShares:   1952,
			SessionRejected: 0,
		},
		Pool: &device.PoolResponse{Host: "hmpool.io", Port: 3337, Connected: true},
	})
	m = updated.(Model)

	hr2 := 1_780_000_000_000.0 // 1.78 TH/s
	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Stats: &device.StatsResponse{
			Hashrate:        hr2,
			TempC:           61.0,
			SessionShares:   22004,
			SessionRejected: 3,
		},
		Pool: &device.PoolResponse{Host: "digi.hmpool.io", Port: 3334, Connected: true},
	})
	m = updated.(Model)
	return m
}

// ── initial state ─────────────────────────────────────────────────────────────

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.True(t, m.discovering)
	assert.False(t, m.ready)
	assert.Empty(t, m.targets)
	assert.Empty(t, m.state)
	assert.Equal(t, 0, m.selected)
}

// ── phase rendering ───────────────────────────────────────────────────────────

func TestModel_ViewBeforeData(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := updated.(Model).View()
	assert.Contains(t, view, "Discovering")
	assert.NotPanics(t, func() { _ = view })
}

func TestModel_DiscoveredMsg_TransitionsToMonitoring(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, cmd := m.Update(discoveredMsg{targets: makeTargets()})
	m2 := updated.(Model)
	assert.False(t, m2.discovering)
	assert.Nil(t, m2.discoverErr)
	assert.Len(t, m2.targets, 2)
	assert.NotNil(t, cmd) // should return Batch(refreshAll, tickCmd)
}

func TestModel_DiscoveredMsg_Error(t *testing.T) {
	m := NewModel(stubDiscover(nil, fmt.Errorf("mdns failed")))
	updated, cmd := m.Update(discoveredMsg{err: fmt.Errorf("mdns failed")})
	m2 := updated.(Model)
	assert.False(t, m2.discovering)
	assert.NotNil(t, m2.discoverErr)
	assert.Nil(t, cmd)
	view := m2.View()
	assert.Contains(t, view, "Error")
	assert.Contains(t, view, "press q to quit")
}

func TestModel_DiscoveredMsg_Empty(t *testing.T) {
	m := NewModel(stubDiscover(nil, nil))
	updated, cmd := m.Update(discoveredMsg{targets: nil})
	m2 := updated.(Model)
	assert.False(t, m2.discovering)
	assert.Nil(t, m2.discoverErr)
	assert.Empty(t, m2.targets)
	assert.Nil(t, cmd)
	view := m2.View()
	assert.Contains(t, view, "No devices found")
	assert.Contains(t, view, "press q to quit")
}

func TestModel_ViewPhases_NoPanic(t *testing.T) {
	// discovering phase
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.NotPanics(t, func() { _ = m.View() })

	// error phase
	updated, _ := m.Update(discoveredMsg{err: fmt.Errorf("boom")})
	assert.NotPanics(t, func() { _ = updated.(Model).View() })

	// empty phase
	m2 := NewModel(stubDiscover(nil, nil))
	updated2, _ := m2.Update(discoveredMsg{targets: nil})
	assert.NotPanics(t, func() { _ = updated2.(Model).View() })

	// querying phase (targets present, ready false)
	m3 := NewModel(stubDiscover(makeTargets(), nil))
	updated3, _ := m3.Update(discoveredMsg{targets: makeTargets()})
	assert.NotPanics(t, func() { _ = updated3.(Model).View() })

	// ready phase (targets present, ready true)
	m4 := NewModel(stubDiscover(makeTargets(), nil))
	updated4, _ := m4.Update(discoveredMsg{targets: makeTargets()})
	m4 = updated4.(Model)
	updated4, _ = m4.Update(PolledMsg{Host: "miner-a", Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50}})
	m4 = updated4.(Model)
	updated4, _ = m4.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50}})
	assert.NotPanics(t, func() { _ = updated4.(Model).View() })
}

// ── fleet banner ──────────────────────────────────────────────────────────────

func TestModel_View_FleetBanner_TwoOnline(t *testing.T) {
	m := setupTwoMiners(t)
	view := m.View()
	assert.Contains(t, view, "FLEET")
	assert.Contains(t, view, "2 online")
	// both pool host lines present in banner (no port)
	assert.Contains(t, view, "hmpool.io")
	assert.Contains(t, view, "digi.hmpool.io")
	// both hostnames in rows
	assert.Contains(t, view, "miner-a")
	assert.Contains(t, view, "miner-b")
	// pool host:port NOT in row lines (only in banner)
	assert.NotContains(t, view, "hmpool.io:3337")
	assert.NotContains(t, view, "digi.hmpool.io:3334")
	// header contains column titles
	assert.Contains(t, view, "Host")
	assert.Contains(t, view, "Board")
	assert.Contains(t, view, "Version")
}

func TestModel_View_FleetBanner_PoolHashrateSummed(t *testing.T) {
	// two miners on same pool — hashrate should be summed
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	for _, host := range []string{"miner-a", "miner-b"} {
		updated, _ = m.Update(PolledMsg{
			Host: host,
			Stats: &device.StatsResponse{
				Hashrate: 1_000_000_000_000.0, // 1 TH/s each
			},
			Pool: &device.PoolResponse{Host: "pool.example.com", Port: 3333, Connected: true},
		})
		m = updated.(Model)
	}

	_, pools := fleetSummary(m)
	require.Len(t, pools, 1)
	assert.InDelta(t, 2_000_000_000_000.0, pools[0].hashrate, 1)
	assert.Equal(t, 2, pools[0].count)
	assert.Equal(t, "pool.example.com", pools[0].host)
}

func TestModel_View_FleetBanner_PoolsSortedByHashrateDesc(t *testing.T) {
	m := setupTwoMiners(t)
	_, pools := fleetSummary(m)
	require.Len(t, pools, 2)
	// digi pool has higher hashrate (1.78 TH/s) → should be first
	assert.Equal(t, "digi.hmpool.io", pools[0].host)
	assert.Equal(t, "hmpool.io", pools[1].host)
}

func TestModel_View_FleetBanner_SameHostDifferentPorts_Combined(t *testing.T) {
	// two miners on same host but different ports — should combine into one line
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1_000_000_000_000.0}, // 1 TH/s
		Pool: &device.PoolResponse{Host: "pool.example.com", Port: 3333, Connected: true},
	})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Stats: &device.StatsResponse{Hashrate: 500_000_000_000.0}, // 0.5 TH/s
		Pool: &device.PoolResponse{Host: "pool.example.com", Port: 3334, Connected: true},
	})
	m = updated.(Model)

	_, pools := fleetSummary(m)
	require.Len(t, pools, 1)
	assert.Equal(t, "pool.example.com", pools[0].host)
	assert.InDelta(t, 1.5e12, pools[0].hashrate, 1)
	assert.Equal(t, 2, pools[0].count)
}

// ── pool host:port in banner but not row ──────────────────────────────────────

func TestModel_PoolHostPort_InBannerNotInRow(t *testing.T) {
	// Pool host:port should appear in banner, not in row lines
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{
			Hashrate:        1_234_000_000.0,
			TempC:           52.0,
			SessionShares:   100,
			SessionRejected: 2,
		},
		Pool: &device.PoolResponse{Host: "pool.unique-test.com", Port: 9999, Connected: true},
	})
	m = updated.(Model)

	// Need both to reach ready state
	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Stats: &device.StatsResponse{
			Hashrate:        500_000_000.0,
			TempC:           50.0,
			SessionShares:   50,
			SessionRejected: 1,
		},
		Pool: &device.PoolResponse{Host: "other.pool.com", Port: 3333, Connected: true},
	})
	m = updated.(Model)

	view := m.View()
	// Pool host in banner
	assert.Contains(t, view, "pool.unique-test.com")
	// Pool host:port NOT in row
	assert.NotContains(t, view, "pool.unique-test.com:9999")
}

// ── offline device ────────────────────────────────────────────────────────────

func TestModel_PolledMsg_StoresState(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	hashrate := 500_000.0
	msg := PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{
			Hashrate:        hashrate,
			TempC:           45.0,
			SessionShares:   5,
			SessionRejected: 1,
		},
		Pool: &device.PoolResponse{
			Host:      "pool.example.com",
			Connected: true,
		},
	}

	updated, _ = m.Update(msg)
	m2 := updated.(Model)

	st, ok := m2.state["miner-a"]
	require.True(t, ok)
	assert.NoError(t, st.err)
	require.NotNil(t, st.stats)
	assert.InDelta(t, hashrate, st.stats.Hashrate, 0.1)
}

func TestModel_OfflineDevice_ExcludedFromPoolLines(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// miner-a online
	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50},
		Pool: &device.PoolResponse{Host: "pool.example.com", Port: 3333, Connected: true},
	})
	m = updated.(Model)

	// miner-b offline
	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Err:  fmt.Errorf("connection refused"),
	})
	m = updated.(Model)

	view := m.View()
	// 2 miners total but only 1 online
	assert.Contains(t, view, "2 miner")
	assert.Contains(t, view, "1 online")
	// offline row shows "offline"
	assert.Contains(t, view, "offline")
	// pool line present for online miner (host only, no port)
	assert.Contains(t, view, "pool.example.com")

	// pool summary only contains online miner's pool
	online, pools := fleetSummary(m)
	assert.Equal(t, 1, online)
	assert.Len(t, pools, 1)
}

func TestModel_OfflineRow_WhenErrSet(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Err:  fmt.Errorf("connection refused"),
	})
	m = updated.(Model)

	// Need both targets to respond before ready
	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Err:  fmt.Errorf("connection refused"),
	})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view, "offline")
}

// ── selection / navigation ────────────────────────────────────────────────────

func TestModel_UpDown_MovesSelected(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	assert.Equal(t, 0, m.selected)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	assert.Equal(t, 1, m.selected)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(Model)
	assert.Equal(t, 0, m.selected)

	// down arrow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	assert.Equal(t, 1, m.selected)

	// up arrow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	assert.Equal(t, 0, m.selected)
}

func TestModel_UpDown_ClampsAtBounds(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// clamp at top
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(Model)
	assert.Equal(t, 0, m.selected)

	// move to bottom
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	// clamp at bottom
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	assert.Equal(t, 1, m.selected) // len=2, max index=1
}

func TestModel_SelectedRow_StyledDifferently(t *testing.T) {
	m := setupTwoMiners(t)

	// selected=0 (miner-a selected)
	view0 := m.View()

	// move to miner-b
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := updated.(Model)
	assert.Equal(t, 1, m2.selected)

	view1 := m2.View()
	// The views should differ because selection changed
	assert.NotEqual(t, view0, view1)
}

// ── view contains hostname after data ─────────────────────────────────────────

func TestModel_ViewAfterData_ContainsHostname(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{
			Hashrate:        1_234_000,
			TempC:           50.0,
			SessionShares:   10,
			SessionRejected: 0,
		},
		Pool: &device.PoolResponse{Host: "pool.example.com", Connected: true},
	})
	m = updated.(Model)

	// Need both targets to respond before table renders
	updated, _ = m.Update(PolledMsg{
		Host: "miner-b",
		Stats: &device.StatsResponse{
			Hashrate:        1_234_000,
			TempC:           50.0,
			SessionShares:   10,
			SessionRejected: 0,
		},
		Pool: &device.PoolResponse{Host: "pool.example.com", Connected: true},
	})
	m = updated.(Model)

	view := m.View()
	assert.Contains(t, view, "miner-a")
	assert.Contains(t, view, "MH/s")
	assert.Contains(t, view, "v1.2.3")
}

func TestModel_ViewAfterData_NoPanic(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	_, _ = m.Update(discoveredMsg{targets: makeTargets()})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e6, TempC: 45},
	})
	assert.NotPanics(t, func() { _ = m.View() })
}

// ── keys ──────────────────────────────────────────────────────────────────────

func TestModel_QuitKey_ReturnsQuitCmd(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.Equal(t, tea.Quit(), msg)
}

func TestModel_QuitKey_WorksDuringDiscovery(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.True(t, m.discovering)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.Equal(t, tea.Quit(), msg)
}

func TestModel_WindowSizeMsg_NoPanic(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.NotPanics(t, func() {
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	})
	assert.NotPanics(t, func() {
		m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	})
}

func TestModel_SpinnerTick_IgnoredAfterReady(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)
	m.ready = true
	m.discovering = false

	tickMsg := spinner.TickMsg{Time: time.Now(), ID: m.spin.ID()}
	updated, cmd := m.Update(tickMsg)
	m2 := updated.(Model)
	assert.True(t, m2.ready)
	assert.Nil(t, cmd)
}

func TestModel_RefreshKey_IgnoredDuringDiscovery(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.True(t, m.discovering)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.Nil(t, cmd)
}

func TestModel_RefreshKey_WorksAfterDiscovery(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.NotNil(t, cmd)
}

// ── AsicHashrate / AsicTempC preferred ────────────────────────────────────────

func TestModel_AsicHashrate_UsedWhenPresent(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	asicHR := 2_000_000_000_000.0
	updated, _ = m.Update(PolledMsg{
		Host: "miner-a",
		Stats: &device.StatsResponse{
			Hashrate:     100_000_000_000.0, // lower base
			AsicHashrate: &asicHR,
			TempC:        50,
		},
		Pool: &device.PoolResponse{Host: "pool.example.com", Port: 3333},
	})
	m = updated.(Model)

	_, pools := fleetSummary(m)
	require.Len(t, pools, 1)
	assert.InDelta(t, asicHR, pools[0].hashrate, 1)
}

// ── incremental polling / ready state ─────────────────────────────────────────

func TestModel_TwoTargets_FirstPolledMsg_ShowsQuerying(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// After first PolledMsg (miner-a), ready should be false (only 1 of 2 targets reported)
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50},
	})
	m = updated.(Model)

	assert.False(t, m.ready)
	assert.Len(t, m.state, 1)
	view := m.View()
	assert.Contains(t, view, "Querying 2 miners")
	assert.NotContains(t, view, "FLEET")
	assert.NotContains(t, view, "miner-a")
	assert.NotContains(t, view, "miner-b")
}

func TestModel_TwoTargets_SecondPolledMsg_ShowsTable(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// First PolledMsg
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50},
	})
	m = updated.(Model)
	assert.False(t, m.ready)

	// Second PolledMsg — ready becomes true
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-b",
		Stats: &device.StatsResponse{Hashrate: 1.5e12, TempC: 55},
	})
	m = updated.(Model)

	assert.True(t, m.ready)
	assert.Len(t, m.state, 2)
	view := m.View()
	assert.Contains(t, view, "FLEET")
	assert.Contains(t, view, "miner-a")
	assert.Contains(t, view, "miner-b")
	assert.NotContains(t, view, "Querying")
}

func TestModel_Ready_Persists_AfterLaterTick(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// Get to ready state
	updated, _ = m.Update(PolledMsg{Host: "miner-a", Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50}})
	m = updated.(Model)
	updated, _ = m.Update(PolledMsg{Host: "miner-b", Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50}})
	m = updated.(Model)
	assert.True(t, m.ready)

	// Send a tick (simulating refresh) — ready should stay true
	updated, _ = m.Update(tickMsg(time.Now()))
	m = updated.(Model)
	assert.True(t, m.ready)

	view := m.View()
	assert.Contains(t, view, "FLEET")
	assert.NotContains(t, view, "Querying")
}

func TestModel_QuitKey_WorksDuringQuerying(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	// Send one PolledMsg — not ready yet
	updated, _ = m.Update(PolledMsg{Host: "miner-a", Stats: &device.StatsResponse{Hashrate: 1e12, TempC: 50}})
	m = updated.(Model)
	assert.False(t, m.ready)

	// q should still quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.Equal(t, tea.Quit(), msg)
}

// ── cmd constructors ──────────────────────────────────────────────────────────

func TestTickCmd_InnerFunc(t *testing.T) {
	cmd := tickCmd()
	require.NotNil(t, cmd)
	// Execute the returned Cmd func — should return a tickMsg.
	msg := cmd()
	_, ok := msg.(tickMsg)
	assert.True(t, ok, "tickCmd inner func must return tickMsg")
}

func TestDiscoverCmd_CallsFn(t *testing.T) {
	targets := makeTargets()
	cmd := discoverCmd(stubDiscover(targets, nil))
	require.NotNil(t, cmd)
	raw := cmd()
	dm, ok := raw.(discoveredMsg)
	require.True(t, ok)
	assert.Nil(t, dm.err)
	assert.Len(t, dm.targets, len(targets))
}

func TestDiscoverCmd_PropagatesError(t *testing.T) {
	cmd := discoverCmd(stubDiscover(nil, fmt.Errorf("mdns boom")))
	raw := cmd()
	dm, ok := raw.(discoveredMsg)
	require.True(t, ok)
	assert.NotNil(t, dm.err)
	assert.Contains(t, dm.err.Error(), "mdns boom")
}

func TestModel_Init_ReturnsCmd(t *testing.T) {
	m := NewModel(stubDiscover(makeTargets(), nil))
	cmd := m.Init()
	assert.NotNil(t, cmd, "Init must return a non-nil Batch cmd")
}

// ── Update uncovered branches ─────────────────────────────────────────────────

func TestModel_TickMsg_WithNoTargets(t *testing.T) {
	// tickMsg when targets is empty (discovery done, 0 found) — hits line 107 return tickCmd() branch.
	m := NewModel(stubDiscover(nil, nil))
	updated, _ := m.Update(discoveredMsg{targets: nil})
	m = updated.(Model)
	assert.Empty(t, m.targets)

	updated, cmd := m.Update(tickMsg(time.Now()))
	m2 := updated.(Model)
	assert.Empty(t, m2.targets)
	assert.NotNil(t, cmd, "tickMsg with no targets must return a tickCmd")
}

func TestModel_SpinnerTick_WhileDiscovering(t *testing.T) {
	// spinner.TickMsg while m.discovering=true — hits the spin.Update path (lines 122-126).
	m := NewModel(stubDiscover(makeTargets(), nil))
	assert.True(t, m.discovering)

	stMsg := spinner.TickMsg{Time: time.Now(), ID: m.spin.ID()}
	updated, cmd := m.Update(stMsg)
	m2 := updated.(Model)
	assert.True(t, m2.discovering)
	// cmd should be non-nil (spinner produces a follow-up tick)
	assert.NotNil(t, cmd)
}

func TestModel_SpinnerTick_WhileQueryingNotReady(t *testing.T) {
	// spinner.TickMsg after discovery (discovering=false) but before all polled (ready=false).
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)
	assert.False(t, m.discovering)
	assert.False(t, m.ready)

	stMsg := spinner.TickMsg{Time: time.Now(), ID: m.spin.ID()}
	updated, cmd := m.Update(stMsg)
	m2 := updated.(Model)
	assert.False(t, m2.ready)
	assert.NotNil(t, cmd)
}

func TestModel_EnterEscKeys(t *testing.T) {
	// "enter" and "esc" are reserved (no-ops) — hits lines 148-150.
	m := setupTwoMiners(t)
	for _, key := range []string{"enter", "esc"} {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m2 := updated.(Model)
		assert.Nil(t, cmd, "key %q should produce nil cmd", key)
		assert.Equal(t, m.selected, m2.selected)
	}
}

func TestModel_UnknownMsg_Fallthrough(t *testing.T) {
	// An unrecognised message type hits the final `return m, nil` in Update (line 154).
	type unknownMsg struct{}
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, cmd := m.Update(unknownMsg{})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}

// ── ASIC-preferred path ───────────────────────────────────────────────────────

func makeAsicStats(base, asicHR float64, baseTemp, asicTemp float64, baseShares uint32, asicShares uint32) *device.StatsResponse {
	s := &device.StatsResponse{
		Hashrate:        base,
		TempC:           baseTemp,
		SessionShares:   baseShares,
		SessionRejected: 0,
	}
	s.AsicHashrate = &asicHR
	s.AsicTempC = &asicTemp
	s.AsicShares = &asicShares
	return s
}

func TestModel_RenderHeader_AsicPreferred(t *testing.T) {
	// renderHeader ASIC branches (lines 223-233): AsicHashrate/AsicTempC/AsicShares non-nil.
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	asicHR := 3_500_000_000_000.0
	asicTemp := 72.5
	var asicShares uint32 = 9999
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: makeAsicStats(100e9, asicHR, 50.0, asicTemp, 100, asicShares), //nolint:gomnd
		Pool:  &device.PoolResponse{Host: "pool.example.com"},
	})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host:  "miner-b",
		Stats: makeAsicStats(100e9, asicHR, 50.0, asicTemp, 100, asicShares),
		Pool:  &device.PoolResponse{Host: "pool.example.com"},
	})
	m = updated.(Model)
	require.True(t, m.ready)

	// renderHeader is called in View(); should not panic and should contain ASIC hashrate.
	view := m.View()
	// 3.5 TH/s from ASIC path
	assert.Contains(t, view, "3.50 TH/s")
	// ASIC temp
	assert.Contains(t, view, "72.5°C")
}

func TestModel_RenderRows_AsicPreferred(t *testing.T) {
	// renderRows first-pass ASIC branches (lines 344-354): AsicHashrate/AsicTempC/AsicShares non-nil.
	m := NewModel(stubDiscover(makeTargets(), nil))
	updated, _ := m.Update(discoveredMsg{targets: makeTargets()})
	m = updated.(Model)

	asicHR := 2_000_000_000_000.0
	asicTemp := 68.0
	var asicShares uint32 = 5000
	updated, _ = m.Update(PolledMsg{
		Host:  "miner-a",
		Stats: makeAsicStats(1e9, asicHR, 40.0, asicTemp, 10, asicShares),
		Pool:  &device.PoolResponse{Host: "pool.example.com"},
	})
	m = updated.(Model)

	updated, _ = m.Update(PolledMsg{
		Host:  "miner-b",
		Stats: makeAsicStats(1e9, asicHR, 40.0, asicTemp, 10, asicShares),
		Pool:  &device.PoolResponse{Host: "pool.example.com"},
	})
	m = updated.(Model)
	require.True(t, m.ready)

	view := m.View()
	// ASIC hashrate should be shown (not base 1 GH/s)
	assert.Contains(t, view, "2.00 TH/s")
	assert.Contains(t, view, "68.0°C")
}

// ── fmtHashrate edge cases ────────────────────────────────────────────────────

func TestFmtHashrate_AllBranches(t *testing.T) {
	cases := []struct {
		input    float64
		contains string
	}{
		{2e12, "TH/s"},
		{5e9, "GH/s"},
		{3e6, "MH/s"},
		{500e3, "kH/s"},
		{42.0, "H/s"},
	}
	for _, tc := range cases {
		out := fmtHashrate(tc.input)
		assert.Contains(t, out, tc.contains, "fmtHashrate(%g)", tc.input)
	}
}

func TestFmtHashrate_SubKilo(t *testing.T) {
	out := fmtHashrate(1.5)
	assert.Contains(t, out, "H/s")
	assert.NotContains(t, out, "kH/s")
}
