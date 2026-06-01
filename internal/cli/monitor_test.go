package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/tui"
)

func resetMonitorFlags() {
	monitorAll = false
	monitorHosts = nil
	monitorTimeout = 5
}

// TestRunMonitor_NonTTY verifies the TTY guard fires and runMonitorProgram is not called.
func TestRunMonitor_NonTTY(t *testing.T) {
	defer resetMonitorFlags()

	orig := monitorIsTTY
	defer func() { monitorIsTTY = orig }()
	monitorIsTTY = func() bool { return false }

	programCalled := false
	origProg := runMonitorProgram
	defer func() { runMonitorProgram = origProg }()
	runMonitorProgram = func(_ tea.Model) error {
		programCalled = true
		return nil
	}

	err := runMonitor(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
	assert.False(t, programCalled, "runMonitorProgram must not be called when not a TTY")
}

// TestRunMonitor_HappyPath verifies the post-TTY-guard path: model is constructed
// and runMonitorProgram is called with a tui.Model when TTY check passes.
// Uses an explicit host to take the fast path in resolveTargets (no mDNS).
// Also calls model.Init() to invoke the discoverFn closure, covering the
// resolveTargets call site inside runMonitor.
func TestRunMonitor_HappyPath(t *testing.T) {
	defer resetMonitorFlags()
	monitorHosts = []string{"miner-test.local"}

	orig := monitorIsTTY
	defer func() { monitorIsTTY = orig }()
	monitorIsTTY = func() bool { return true }

	var capturedModel tea.Model
	origProg := runMonitorProgram
	defer func() { runMonitorProgram = origProg }()
	runMonitorProgram = func(m tea.Model) error {
		capturedModel = m
		// Invoke Init to get the Batch cmd, then execute each sub-cmd to fire
		// the discoverFn closure — covers the resolveTargets call in runMonitor (line 54-56).
		// Explicit host means resolveTargets takes the fast path (no mDNS).
		initCmd := m.Init()
		require.NotNil(t, initCmd)
		result := initCmd()
		if batch, ok := result.(tea.BatchMsg); ok {
			for _, sub := range batch {
				if sub != nil {
					sub()
				}
			}
		}
		return nil
	}

	err := runMonitor(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, capturedModel, "runMonitorProgram must be called with a non-nil model")
	_, ok := capturedModel.(tui.Model)
	assert.True(t, ok, "model must be a tui.Model")
}

// TestRunMonitor_ImplicitAll verifies that when neither --host nor --all are set,
// monitorAll is defaulted to true before the TTY guard fires.
func TestRunMonitor_ImplicitAll(t *testing.T) {
	defer resetMonitorFlags()

	orig := monitorIsTTY
	defer func() { monitorIsTTY = orig }()
	monitorIsTTY = func() bool { return false }

	err := runMonitor(nil, nil)
	require.Error(t, err)
	assert.Equal(t, "monitor requires an interactive terminal", err.Error(),
		"should hit TTY guard, not a missing-host error")
	assert.True(t, monitorAll, "monitorAll should be set when no hosts and no --all")
}
