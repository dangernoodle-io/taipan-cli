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

type statsResult struct {
	stats *device.StatsResponse
	err   error
}

func runStats(cmd *cobra.Command, args []string) error {
	if !statsAll && len(statsHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	if statsJSON {
		ui.SetEnabled(false)
	}

	targetDevices, err := resolveTargets(statsHosts, statsAll, statsTimeout)
	if err != nil {
		return err
	}
	if !statsAll && len(targetDevices) == 0 {
		return fmt.Errorf("no matching devices found")
	}

	results := make([]statsResult, len(targetDevices))
	stop := ui.Single("Querying devices…")

	var wg sync.WaitGroup
	for i, d := range targetDevices {
		wg.Add(1)
		go func(idx int, dev discover.DeviceInfo) {
			defer wg.Done()
			client := device.NewClient(dev.IP, dev.Port)
			s, err := client.Stats(context.Background())
			if err != nil {
				results[idx] = statsResult{err: err}
			} else {
				results[idx] = statsResult{stats: s}
			}
		}(i, d)
	}
	wg.Wait()
	stop()

	var errCount int
	for i, d := range targetDevices {
		r := results[i]
		if r.err != nil {
			output.Error("[%s] %v", d.Hostname, r.err)
			errCount++
			continue
		}
		if statsJSON {
			data, err := json.MarshalIndent(r.stats, "", "  ")
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
			printStats(r.stats)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d of %d device(s) failed", errCount, len(targetDevices))
	}
	return nil
}

func printStats(s *device.StatsResponse) {
	fmt.Printf("  %-16s %s\n", "Hashrate:", fmtHashrate(s.Hashrate))
	fmt.Printf("  %-16s %s\n", "Avg Hashrate:", fmtHashrate(s.HashrateAvg))
	fmt.Printf("  %-16s %.1f°C\n", "Temp:", s.TempC)
	fmt.Printf("  %-16s %d accepted / %d rejected\n", "Shares:", s.SessionShares, s.SessionRejected)

	if s.BestDiff > 0 {
		fmt.Printf("  %-16s %s\n", "Best Diff:", fmtDiff(s.BestDiff))
	} else {
		fmt.Printf("  %-16s --\n", "Best Diff:")
	}

	fmt.Printf("  %-16s %s\n", "Uptime:", fmtUptime(s.UptimeS))
	fmt.Printf("  %-16s %s\n", "Last Share:", fmtLastShare(s.LastShareAgoS))

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
