package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
)

var (
	rebootHosts   []string
	rebootAll     bool
	rebootTimeout int
	rebootForce   bool
)

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot TaipanMiner devices",
	RunE:  runReboot,
}

func init() {
	rebootCmd.Flags().StringArrayVar(&rebootHosts, "host", nil, "Target device hostname (repeatable)")
	rebootCmd.Flags().BoolVar(&rebootAll, "all", false, "Reboot all discovered devices")
	rebootCmd.Flags().IntVarP(&rebootTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	rebootCmd.Flags().BoolVarP(&rebootForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(rebootCmd)
}

func runReboot(cmd *cobra.Command, args []string) error {
	if !rebootAll && len(rebootHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	targetDevices, err := resolveTargets(rebootHosts, rebootAll, rebootTimeout)
	if err != nil {
		return err
	}
	if !rebootAll && len(targetDevices) == 0 {
		return fmt.Errorf("no matching devices found")
	}

	// Confirm unless --force
	if !rebootForce {
		fmt.Fprintf(os.Stderr, "Reboot %d device(s)? [y/N]: ", len(targetDevices))
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			return fmt.Errorf("reboot cancelled")
		}
	}

	type rebootResult struct {
		err error
	}

	results := make([]rebootResult, len(targetDevices))
	g := ui.NewGroup()
	lines := make([]*ui.Line, len(targetDevices))
	for i, d := range targetDevices {
		lines[i] = g.Add("rebooting " + d.Hostname)
	}
	g.Start()

	var wg sync.WaitGroup
	for i, d := range targetDevices {
		wg.Add(1)
		go func(idx int, dev discover.DeviceInfo) {
			defer wg.Done()
			client := device.NewClient(dev.IP, dev.Port)
			_, err := client.Reboot(context.Background())
			if err != nil {
				results[idx] = rebootResult{err: err}
				lines[idx].Error(dev.Hostname + ": " + err.Error())
			} else {
				lines[idx].Complete(dev.Hostname)
			}
		}(i, d)
	}
	wg.Wait()
	g.Stop()

	var errCount int
	for i, d := range targetDevices {
		r := results[i]
		if r.err != nil {
			output.Error("[%s] %v", d.Hostname, r.err)
			errCount++
		} else {
			output.Success("[%s] reboot initiated", d.Hostname)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d of %d device(s) failed", errCount, len(targetDevices))
	}
	return nil
}
