package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
)

// newClient is a var so tests can substitute an httptest-based client by
// constructing DeviceInfo with the test server's host and port.
var newClient = device.NewClient

// PolledMsg carries one device's poll result back to the model.
type PolledMsg struct {
	Host  string
	Stats *device.StatsResponse
	Pool  *device.PoolResponse
	Info  *device.InfoResponse
	Err   error
}

// pollDevice returns a Cmd that queries a single device and emits a PolledMsg.
func pollDevice(d discover.DeviceInfo) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		c := newClient(d.IP, d.Port)

		stats, err := c.Stats(ctx)
		if err != nil {
			return PolledMsg{Host: d.Hostname, Err: err}
		}

		// Pool and Info failures are non-fatal — keep stats.
		pool, _ := c.Pool(ctx)
		info, _ := c.Info(ctx)

		return PolledMsg{Host: d.Hostname, Stats: stats, Pool: pool, Info: info}
	}
}

// refreshAll batches a pollDevice Cmd for every target.
func refreshAll(targets []discover.DeviceInfo) tea.Cmd {
	cmds := make([]tea.Cmd, len(targets))
	for i, d := range targets {
		cmds[i] = pollDevice(d)
	}
	return tea.Batch(cmds...)
}
