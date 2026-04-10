package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

var (
	settingsHost    string
	settingsTimeout int
	settingsJSON    bool
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage device settings",
}

var settingsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get device settings",
	RunE:  runSettingsGet,
}

var settingsSetCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "Set a device setting",
	Args:  cobra.ExactArgs(2),
	RunE:  runSettingsSet,
}

func init() {
	settingsGetCmd.Flags().StringVar(&settingsHost, "host", "", "Target device hostname (required)")
	settingsGetCmd.Flags().IntVarP(&settingsTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	settingsGetCmd.Flags().BoolVar(&settingsJSON, "json", false, "Output as JSON")

	settingsSetCmd.Flags().StringVar(&settingsHost, "host", "", "Target device hostname (required)")
	settingsSetCmd.Flags().IntVarP(&settingsTimeout, "timeout", "t", 5, "Browse timeout in seconds")

	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	rootCmd.AddCommand(settingsCmd)
}

func runSettingsGet(cmd *cobra.Command, args []string) error {
	if settingsHost == "" {
		return fmt.Errorf("--host is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settingsTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	targetDevices := filterDevices(devices, []string{settingsHost})
	if len(targetDevices) == 0 {
		return fmt.Errorf("device not found: %s", settingsHost)
	}

	d := targetDevices[0]
	client := device.NewClient(d.IP, d.Port)
	settings, err := client.GetSettings(context.Background())
	if err != nil {
		return fmt.Errorf("[%s] %w", d.Hostname, err)
	}

	if settingsJSON {
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		printSettings(settings)
	}

	return nil
}

func runSettingsSet(cmd *cobra.Command, args []string) error {
	if settingsHost == "" {
		return fmt.Errorf("--host is required")
	}

	key := args[0]
	valueStr := args[1]

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settingsTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	targetDevices := filterDevices(devices, []string{settingsHost})
	if len(targetDevices) == 0 {
		return fmt.Errorf("device not found: %s", settingsHost)
	}

	d := targetDevices[0]
	client := device.NewClient(d.IP, d.Port)

	// Type coercion based on key
	var value any
	switch key {
	case "display_en", "ota_skip_check":
		v, err := strconv.ParseBool(valueStr)
		if err != nil {
			return fmt.Errorf("invalid boolean value for %s: %w", key, err)
		}
		value = v
	case "pool_port":
		v, err := strconv.Atoi(valueStr)
		if err != nil {
			return fmt.Errorf("invalid integer value for %s: %w", key, err)
		}
		value = v
	default:
		value = valueStr
	}

	response, err := client.SetSetting(context.Background(), key, value)
	if err != nil {
		return fmt.Errorf("[%s] %w", d.Hostname, err)
	}

	if response.RebootRequired {
		output.Success("[%s] setting updated (reboot required)", d.Hostname)
	} else {
		output.Success("[%s] setting updated", d.Hostname)
	}

	return nil
}

func printSettings(s *device.SettingsResponse) {
	fmt.Printf("  %-16s %s\n", "Pool Host:", s.PoolHost)
	fmt.Printf("  %-16s %d\n", "Pool Port:", s.PoolPort)
	fmt.Printf("  %-16s %s\n", "Wallet:", s.Wallet)
	fmt.Printf("  %-16s %s\n", "Worker:", s.Worker)

	if s.PoolPass == "" {
		fmt.Printf("  %-16s %s\n", "Pool Pass:", "(not set)")
	} else {
		fmt.Printf("  %-16s %s\n", "Pool Pass:", "****")
	}

	fmt.Printf("  %-16s %v\n", "Display:", s.DisplayEn)
	fmt.Printf("  %-16s %v\n", "OTA Skip Check:", s.OTASkipCheck)
}
