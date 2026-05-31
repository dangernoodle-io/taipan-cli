package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
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

type poolResult struct {
	pool *device.PoolResponse
	err  error
}

func runPool(cmd *cobra.Command, args []string) error {
	if !poolAll && len(poolHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	if poolJSON {
		ui.SetEnabled(false)
	}

	targetDevices, err := resolveTargets(poolHosts, poolAll, poolTimeout)
	if err != nil {
		return err
	}
	if !poolAll && len(targetDevices) == 0 {
		return fmt.Errorf("no matching devices found")
	}

	results := make([]poolResult, len(targetDevices))
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
			p, err := client.Pool(context.Background())
			if err != nil {
				results[idx] = poolResult{err: err}
				lines[idx].Error(dev.Hostname + ": " + err.Error())
			} else {
				results[idx] = poolResult{pool: p}
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
		if poolJSON {
			data, err := json.MarshalIndent(r.pool, "", "  ")
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
			printPool(r.pool)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d of %d device(s) failed", errCount, len(targetDevices))
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
