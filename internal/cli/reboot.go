package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rebootTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	var targetDevices []discover.DeviceInfo
	if rebootAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, rebootHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
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

	var errs []error
	for _, d := range targetDevices {
		client := device.NewClient(d.IP, d.Port)
		_, err := client.Reboot(context.Background())
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] %w", d.Hostname, err))
			continue
		}
		output.Success("[%s] reboot initiated", d.Hostname)
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
