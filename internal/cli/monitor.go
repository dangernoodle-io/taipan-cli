package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/tui"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
)

var (
	monitorHosts   []string
	monitorAll     bool
	monitorTimeout int
)

// monitorIsTTY and runMonitorProgram are package-level vars so tests can inject stubs.
var monitorIsTTY = func() bool { return isatty.IsTerminal(os.Stdout.Fd()) }
var runMonitorProgram = func(m tea.Model) error {
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Live fleet dashboard (requires an interactive terminal)",
	RunE:  runMonitor,
}

func init() {
	monitorCmd.Flags().StringArrayVar(&monitorHosts, "host", nil, "Target device hostname (repeatable)")
	monitorCmd.Flags().BoolVar(&monitorAll, "all", false, "Monitor all discovered devices")
	monitorCmd.Flags().IntVarP(&monitorTimeout, "timeout", "t", 5, "Discovery timeout in seconds")
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor(_ *cobra.Command, _ []string) error {
	if !monitorAll && len(monitorHosts) == 0 {
		monitorAll = true
	}

	if !monitorIsTTY() {
		return fmt.Errorf("monitor requires an interactive terminal")
	}

	// Disable stderr spinner — TUI owns the screen.
	ui.SetEnabled(false)

	discoverFn := func() ([]discover.DeviceInfo, error) {
		return resolveTargets(monitorHosts, monitorAll, monitorTimeout)
	}
	m := tui.NewModel(discoverFn)
	return runMonitorProgram(m)
}
