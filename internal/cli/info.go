package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
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

func runInfo(cmd *cobra.Command, args []string) error {
	if !infoAll && len(infoHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(infoTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	var targetDevices []discover.DeviceInfo
	if infoAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, infoHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
	}

	var errs []error
	for i, d := range targetDevices {
		client := device.NewClient(d.IP, d.Port)
		info, err := client.Info(context.Background())
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] %w", d.Hostname, err))
			continue
		}

		if infoJSON {
			data, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				errs = append(errs, fmt.Errorf("[%s] %w", d.Hostname, err))
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
			printInfo(info)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func printInfo(info *device.InfoResponse) {
	fmt.Printf("  %-16s %s\n", "Board:", info.Board)
	fmt.Printf("  %-16s %s\n", "Version:", info.Version)
	fmt.Printf("  %-16s %s\n", "IDF Version:", info.IDFVersion)
	fmt.Printf("  %-16s %s\n", "MAC:", info.MAC)
	fmt.Printf("  %-16s %s\n", "Worker:", info.WorkerName)
	fmt.Printf("  %-16s %s\n", "SSID:", info.SSID)
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
