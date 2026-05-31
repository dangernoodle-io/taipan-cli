package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
)

var (
	infoHosts   []string
	infoAll     bool
	infoTimeout int
	infoJSON    bool
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show device info for TaipanMiner devices",
	RunE:  runInfo,
}

func init() {
	infoCmd.Flags().StringArrayVar(&infoHosts, "host", nil, "Target device hostname (repeatable)")
	infoCmd.Flags().BoolVar(&infoAll, "all", false, "Show info for all discovered devices")
	infoCmd.Flags().IntVarP(&infoTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(infoCmd)
}

type infoResult struct {
	info *device.InfoResponse
	err  error
}

func runInfo(cmd *cobra.Command, args []string) error {
	if !infoAll && len(infoHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	if infoJSON {
		ui.SetEnabled(false)
	}

	targetDevices, err := resolveTargets(infoHosts, infoAll, infoTimeout)
	if err != nil {
		return err
	}
	if !infoAll && len(targetDevices) == 0 {
		return fmt.Errorf("no matching devices found")
	}

	results := make([]infoResult, len(targetDevices))
	g := ui.NewGroup()
	lines := make([]*ui.Line, len(targetDevices))
	for i, d := range targetDevices {
		lines[i] = g.Add("querying " + d.Hostname)
	}
	g.Start()

	var wg sync.WaitGroup
	for i, d := range targetDevices {
		wg.Add(1)
		go func(idx int, dev discover.DeviceInfo) {
			defer wg.Done()
			client := device.NewClient(dev.IP, dev.Port)
			info, err := client.Info(context.Background())
			if err != nil {
				results[idx] = infoResult{err: err}
				lines[idx].Error(dev.Hostname + ": " + err.Error())
			} else {
				results[idx] = infoResult{info: info}
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
			continue
		}
		if infoJSON {
			data, err := json.MarshalIndent(r.info, "", "  ")
			if err != nil {
				output.Error("[%s] %v", d.Hostname, err)
				errCount++
				continue
			}
			if len(targetDevices) > 1 {
				output.Info("[%s]", d.Hostname)
			}
			fmt.Println(string(data))
		} else {
			if len(targetDevices) > 1 {
				if i > 0 {
					fmt.Println()
				}
				output.Info("[%s]", d.Hostname)
			}
			printInfo(r.info)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d of %d device(s) failed", errCount, len(targetDevices))
	}
	return nil
}

func printInfo(info *device.InfoResponse) {
	fmt.Printf("  %-16s %s\n", "Board:", info.Board)
	fmt.Printf("  %-16s %s\n", "Version:", info.Version)
	fmt.Printf("  %-16s %s\n", "IDF Version:", info.IDFVersion)
	fmt.Printf("  %-16s %s\n", "MAC:", info.MAC)
	fmt.Printf("  %-16s %s\n", "SSID:", info.Network.SSID)
	fmt.Printf("  %-16s %d\n", "Cores:", info.Cores)
	fmt.Printf("  %-16s %d / %d bytes\n", "Heap:", info.FreeHeap, info.TotalHeap)
	fmt.Printf("  %-16s %d bytes\n", "Flash:", info.FlashSize)
	fmt.Printf("  %-16s %s\n", "Reset Reason:", info.ResetReason)
	fmt.Printf("  %-16s %d\n", "WDT Resets:", info.WDTResets)

	if info.BootTime != nil {
		bootTime := time.Unix(*info.BootTime, 0).Format(time.RFC3339)
		fmt.Printf("  %-16s %s\n", "Boot Time:", bootTime)
	}

	if info.AppSize != nil {
		fmt.Printf("  %-16s %d bytes\n", "App Size:", *info.AppSize)
	}
}
