package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

var (
	discoverTimeout int
	discoverJSON    bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover TaipanMiner devices on the local network",
	RunE:  runDiscover,
}

func init() {
	discoverCmd.Flags().IntVarP(&discoverTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	discoverCmd.Flags().BoolVar(&discoverJSON, "json", false, "Output as JSON")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(discoverTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	if discoverJSON {
		return printJSON(devices)
	}

	printTable(devices)
	return nil
}

func printJSON(devices []discover.DeviceInfo) error {
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printTable(devices []discover.DeviceInfo) {
	if len(devices) == 0 {
		output.Warn("No devices found")
		return
	}

	fmt.Printf("%-30s %-15s %-15s %-10s %-15s %-20s\n",
		"Hostname", "IP", "Board", "Version", "Worker", "MAC")
	fmt.Println(string(make([]byte, 120-20)))

	for _, d := range devices {
		fmt.Printf("%-30s %-15s %-15s %-10s %-15s %-20s\n",
			d.Hostname, d.IP, d.Board, d.Version, d.Worker, d.MAC)
	}
}
