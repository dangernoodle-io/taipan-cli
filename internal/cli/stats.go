package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

var (
	statsHosts   []string
	statsAll     bool
	statsTimeout int
	statsJSON    bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show mining stats for TaipanMiner devices",
	RunE:  runStats,
}

func init() {
	statsCmd.Flags().StringArrayVar(&statsHosts, "host", nil, "Target device hostname (repeatable)")
	statsCmd.Flags().BoolVar(&statsAll, "all", false, "Show stats for all discovered devices")
	statsCmd.Flags().IntVarP(&statsTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
	if !statsAll && len(statsHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(statsTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	var targetDevices []discover.DeviceInfo
	if statsAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, statsHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
	}

	var errs []error
	for i, d := range targetDevices {
		client := device.NewClient(d.IP, d.Port)
		stats, err := client.Stats(context.Background())
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] %w", d.Hostname, err))
			continue
		}

		if statsJSON {
			data, err := json.MarshalIndent(stats, "", "  ")
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
			printStats(stats)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func printStats(s *device.StatsResponse) {
	fmt.Printf("  %-16s %s\n", "Hashrate:", fmtHashrate(s.Hashrate))
	fmt.Printf("  %-16s %s\n", "Avg Hashrate:", fmtHashrate(s.HashrateAvg))
	fmt.Printf("  %-16s %.1f°C\n", "Temp:", s.TempC)
	fmt.Printf("  %-16s %d accepted / %d rejected\n", "Shares:", s.SessionShares, s.SessionRejected)

	if s.BestDiff > 0 && s.PoolDifficulty > 0 {
		mult := math.Floor(s.BestDiff / s.PoolDifficulty)
		fmt.Printf("  %-16s %s (%.0fx)\n", "Best Diff:", fmtDiff(s.BestDiff), mult)
	} else if s.BestDiff > 0 {
		fmt.Printf("  %-16s %s\n", "Best Diff:", fmtDiff(s.BestDiff))
	} else {
		fmt.Printf("  %-16s --\n", "Best Diff:")
	}

	fmt.Printf("  %-16s %s\n", "Pool Diff:", fmtPoolDiff(s.PoolDifficulty))
	fmt.Printf("  %-16s %s\n", "Uptime:", fmtUptime(s.UptimeS))
	fmt.Printf("  %-16s %s:%d\n", "Pool:", s.PoolHost, s.PoolPort)
	fmt.Printf("  %-16s %s\n", "Worker:", s.Worker)
	fmt.Printf("  %-16s %s\n", "Last Share:", fmtLastShare(s.LastShareAgoS))
	fmt.Printf("  %-16s %.0f\n", "Lifetime:", s.LifetimeShares)

	if s.AsicHashrate != nil {
		fmt.Printf("  %-16s %s\n", "ASIC Hashrate:", fmtHashrate(*s.AsicHashrate))
	}
	if s.AsicHashrateAvg != nil {
		fmt.Printf("  %-16s %s\n", "ASIC Avg:", fmtHashrate(*s.AsicHashrateAvg))
	}
	if s.AsicTempC != nil {
		fmt.Printf("  %-16s %.1f°C\n", "ASIC Temp:", *s.AsicTempC)
	}
	if s.AsicShares != nil {
		fmt.Printf("  %-16s %d\n", "ASIC Shares:", *s.AsicShares)
	}
}

func fmtHashrate(h float64) string {
	switch {
	case h >= 1e12:
		return fmt.Sprintf("%.2f TH/s", h/1e12)
	case h >= 1e9:
		return fmt.Sprintf("%.2f GH/s", h/1e9)
	case h >= 1e6:
		return fmt.Sprintf("%.2f MH/s", h/1e6)
	case h >= 1e3:
		return fmt.Sprintf("%.2f kH/s", h/1e3)
	default:
		return fmt.Sprintf("%.2f H/s", h)
	}
}

func fmtDiff(d float64) string {
	switch {
	case d >= 1e12:
		return fmt.Sprintf("%.2fT", d/1e12)
	case d >= 1e9:
		return fmt.Sprintf("%.2fG", d/1e9)
	case d >= 1e6:
		return fmt.Sprintf("%.2fM", d/1e6)
	case d >= 1e3:
		return fmt.Sprintf("%.2fk", d/1e3)
	default:
		return fmt.Sprintf("%.4f", d)
	}
}

func fmtPoolDiff(d float64) string {
	if d <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.4f", d)
}

func fmtUptime(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func fmtLastShare(agoS int64) string {
	if agoS < 0 {
		return "never"
	}
	d := time.Duration(agoS) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds ago", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds ago", m, s)
	}
	return fmt.Sprintf("%ds ago", s)
}
