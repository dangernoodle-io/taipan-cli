package cli

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/ota"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

var (
	updateHosts   []string
	updateAll     bool
	updateTimeout int
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update firmware on TaipanMiner devices",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringArrayVar(&updateHosts, "host", nil, "Target device hostname (repeatable)")
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all discovered devices")
	updateCmd.Flags().IntVarP(&updateTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Validate: require --all or at least one --host
	if !updateAll && len(updateHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(updateTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	// Sort devices alphabetically by hostname
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	// Filter by --host if specified
	var targetDevices []discover.DeviceInfo
	if updateAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, updateHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
	}

	// Update each device serially. Surface every per-device failure inline so
	// a miner that errors early (e.g. /api/ota/check 500) doesn't silently
	// disappear from the output behind the single "Error: ..." tail.
	var updateErrors []error
	for _, device := range targetDevices {
		if err := updateDevice(device); err != nil {
			output.Error("%v", err)
			updateErrors = append(updateErrors, err)
		}
	}

	if len(updateErrors) > 0 {
		return fmt.Errorf("%d of %d device(s) failed to update",
			len(updateErrors), len(targetDevices))
	}

	return nil
}

func filterDevices(devices []discover.DeviceInfo, requestedHosts []string) []discover.DeviceInfo {
	var result []discover.DeviceInfo
	foundMap := make(map[string]bool)

	for _, device := range devices {
		deviceHostname := strings.TrimSuffix(device.Hostname, ".")
		for _, requested := range requestedHosts {
			requestedHostname := strings.TrimSuffix(requested, ".")
			if strings.EqualFold(deviceHostname, requestedHostname) {
				result = append(result, device)
				foundMap[strings.ToLower(requestedHostname)] = true
				break
			}
		}
	}

	// Warn for any requested hostname not found
	for _, requested := range requestedHosts {
		requestedHostname := strings.TrimSuffix(requested, ".")
		if !foundMap[strings.ToLower(requestedHostname)] {
			output.Warn("Device not found: %s", requestedHostname)
		}
	}

	return result
}

func updateDevice(device discover.DeviceInfo) error {
	hostname := device.Hostname
	client := ota.NewClient(device.IP, device.Port)

	// Check for available updates
	checkResult, err := client.Check(context.Background())
	if err != nil {
		return fmt.Errorf("[%s] check failed: %w", hostname, err)
	}

	currentVersion := checkResult.CurrentVersion
	latestVersion := checkResult.LatestVersion

	output.Info("[%s] current: %s, latest: %s", hostname, currentVersion, latestVersion)

	// If no update available, return success
	if !checkResult.UpdateAvailable {
		output.Success("[%s] already up to date (%s)", hostname, currentVersion)
		return nil
	}

	output.Info("[%s] updating %s → %s", hostname, currentVersion, latestVersion)

	// Trigger the update
	triggerResult, statusCode, err := client.Trigger(context.Background())
	if err != nil {
		return fmt.Errorf("[%s] trigger failed: %w", hostname, err)
	}

	// Handle status codes
	switch statusCode {
	case 202: // Continue to poll
		// Proceed to poll loop
	case 200: // Already up to date
		output.Success("[%s] update already completed (%s)", hostname, latestVersion)
		return nil
	case 409: // Update already in progress
		output.Warn("[%s] update already in progress, polling status", hostname)
		// Continue to poll loop
	default:
		if triggerResult != nil && triggerResult.Error != "" {
			return fmt.Errorf("[%s] trigger returned status %d: %s", hostname, statusCode, triggerResult.Error)
		}
		return fmt.Errorf("[%s] trigger returned unexpected status %d", hostname, statusCode)
	}

	// Poll for completion with 5min timeout and 2s interval
	pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// The firmware reports state=idle up until the OTA worker task gets
	// scheduled after the trigger returns, so the first poll can race and
	// report idle:0% before any work has started. Only trust the idle-means-
	// done signal once we've observed at least one non-idle status.
	sawProgress := false

	for {
		select {
		case <-pollCtx.Done():
			fmt.Printf("\n")
			return fmt.Errorf("[%s] update polling timeout", hostname)

		case <-ticker.C:
			status, err := client.PollStatus(pollCtx)

			// Connection error during poll likely means device restarted
			if err != nil {
				// Check if it's a network error (connection refused, etc.)
				if isNetworkError(err) {
					fmt.Printf("\n")
					output.Success("[%s] update completed (%s)", hostname, latestVersion)
					return nil
				}
				fmt.Printf("\n")
				return fmt.Errorf("[%s] poll failed: %w", hostname, err)
			}

			// Print progress
			fmt.Printf("\r[%s] %s: %.0f%%\033[K", hostname, status.State, status.ProgressPct)

			if status.InProgress || status.State != "idle" {
				sawProgress = true
			}

			// Check completion states
			if status.State == "complete" {
				fmt.Printf("\n")
				reportBootedVersion(client, hostname, latestVersion)
				return nil
			}

			// Check for error state
			if status.State == "error" {
				fmt.Printf("\n")
				return fmt.Errorf("[%s] update failed: %s", hostname, status.LastError)
			}

			// If not in progress and idle with no error, consider it success —
			// but only after we've seen the worker transition into a
			// non-idle state at least once, to avoid a first-poll race.
			if sawProgress && !status.InProgress && status.State == "idle" && status.LastError == "" {
				fmt.Printf("\n")
				reportBootedVersion(client, hostname, latestVersion)
				return nil
			}
		}
	}
}

// reportBootedVersion polls /api/version after an OTA completes so the user
// sees the version the device actually rebooted into, not just the cached
// pre-update "latest" string. Falls back to the expected value if the device
// doesn't come back before the deadline.
func reportBootedVersion(client *ota.Client, hostname, expected string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	booted, err := client.WaitForBoot(ctx, 2*time.Second)
	if err != nil || booted == "" {
		output.Success("[%s] update completed (expected %s; device not yet responding)", hostname, expected)
		return
	}
	expNorm := strings.TrimPrefix(expected, "v")
	bootNorm := strings.TrimPrefix(booted, "v")
	if bootNorm != expNorm {
		output.Warn("[%s] update completed but booted %s (expected %s)", hostname, booted, expected)
		return
	}
	output.Success("[%s] update completed (%s)", hostname, booted)
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for connection refused or other network-level errors
	if _, ok := err.(net.Error); ok {
		return true
	}

	// Check error message strings for common network errors
	errMsg := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"network unreachable",
		"host unreachable",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(strings.ToLower(errMsg), netErr) {
			return true
		}
	}

	return false
}
