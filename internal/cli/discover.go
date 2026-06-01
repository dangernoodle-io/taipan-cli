package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
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
	if discoverJSON {
		ui.SetEnabled(false)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(discoverTimeout)*time.Second)
	defer cancel()

	stop := ui.Single("Discovering devices…")
	devices, err := discover.Browse(ctx)
	stop()
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

	hw, iw, bw, vw := len("Hostname"), len("IP"), len("Board"), len("Version")
	for _, d := range devices {
		hw = max(hw, len(d.Hostname))
		iw = max(iw, len(d.IP))
		bw = max(bw, len(d.Board))
		vw = max(vw, len(d.Version))
	}

	// pad each column to its content width + 7 spaces; last column unpadded
	const gap = 7
	rowFmt := fmt.Sprintf("%%-%ds%%-%ds%%-%ds%%s\n", hw+gap, iw+gap, bw+gap)
	fmt.Printf(rowFmt, "Hostname", "IP", "Board", "Version")
	fmt.Println(strings.Repeat("-", hw+iw+bw+vw+gap*3))
	for _, d := range devices {
		fmt.Printf(rowFmt, d.Hostname, d.IP, d.Board, d.Version)
	}
}
