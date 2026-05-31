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
	poolHosts   []string
	poolAll     bool
	poolTimeout int
	poolJSON    bool
)

var poolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Show pool connection info for TaipanMiner devices",
	RunE:  runPool,
}

func init() {
	poolCmd.Flags().StringArrayVar(&poolHosts, "host", nil, "Target device hostname (repeatable)")
	poolCmd.Flags().BoolVar(&poolAll, "all", false, "Show pool info for all discovered devices")
	poolCmd.Flags().IntVarP(&poolTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	poolCmd.Flags().BoolVar(&poolJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(poolCmd)
}

func runPool(cmd *cobra.Command, args []string) error {
	if !poolAll && len(poolHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(poolTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	var targetDevices []discover.DeviceInfo
	if poolAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, poolHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
	}

	var errs []error
	for i, d := range targetDevices {
		client := device.NewClient(d.IP, d.Port)
		pool, err := client.Pool(context.Background())
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] %w", d.Hostname, err))
			continue
		}

		if poolJSON {
			data, err := json.MarshalIndent(pool, "", "  ")
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
			printPool(pool)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func printPool(p *device.PoolResponse) {
	fmt.Printf("  %-16s %s:%d\n", "Pool:", p.Host, p.Port)
	fmt.Printf("  %-16s %v\n", "Connected:", p.Connected)
	fmt.Printf("  %-16s %s\n", "Worker:", p.Worker)
	fmt.Printf("  %-16s %s\n", "Wallet:", p.Wallet)
	fmt.Printf("  %-16s %s\n", "Difficulty:", fmtDiff(p.CurrentDifficulty))
	fmt.Printf("  %-16s %s\n", "Pool Hashrate:", fmtHashrate(p.PoolEffectiveHashrate))

	if len(p.Stats) > 0 {
		fmt.Printf("  %-16s %s\n", "Best Diff:", fmtDiff(p.Stats[0].BestDiff))
	}

	fmt.Printf("  %-16s %d\n", "Blocks Found:", p.LifetimeBlocksTotal)

	if p.LatencyMs != nil {
		fmt.Printf("  %-16s %d ms\n", "Latency:", *p.LatencyMs)
	}

	if p.SessionStartAgoS != nil {
		fmt.Printf("  %-16s %s ago\n", "Session:", fmtUptime(float64(*p.SessionStartAgoS)))
	}
}
