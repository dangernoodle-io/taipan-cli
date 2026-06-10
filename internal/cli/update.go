package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/ota"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
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

	targetDevices, err := resolveTargets(updateHosts, updateAll, updateTimeout)
	if err != nil {
		return err
	}
	if !updateAll && len(targetDevices) == 0 {
		return fmt.Errorf("no matching devices found")
	}

	// Update each device serially. Surface every per-device failure inline so
	// a miner that errors early doesn't silently disappear from the output.
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

	// Best-effort pre-check (30s backstop). Boot-mode devices may not expose
	// the check routes — if unavailable, proceed directly to Trigger.
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer checkCancel()
	stopCheck := ui.Single("checking " + hostname)
	checkResult, checkErr := client.Check(checkCtx)
	stopCheck()

	checkUnavailable := errors.Is(checkErr, ota.ErrCheckUnavailable)

	if checkErr != nil && !checkUnavailable {
		return fmt.Errorf("[%s] check failed: %w", hostname, checkErr)
	}

	// If check succeeded, honour early-return outcomes.
	if checkResult != nil {
		currentVersion := checkResult.CurrentVersion
		latestVersion := checkResult.LatestVersion
		output.Info("[%s] current: %s, latest: %s", hostname, currentVersion, latestVersion)

		switch checkResult.Outcome {
		case "available":
			output.Info("[%s] updating %s → %s", hostname, currentVersion, latestVersion)
		case "up_to_date":
			output.Success("[%s] already up to date (%s)", hostname, currentVersion)
			return nil
		case "no_asset":
			output.Warn("[%s] no firmware published for this board", hostname)
			return nil
		case "check_failed":
			return fmt.Errorf("[%s] update check failed (device could not complete the release check)", hostname)
		default:
			return fmt.Errorf("[%s] unexpected update check outcome %q", hostname, checkResult.Outcome)
		}
	} else {
		output.Info("[%s] pre-check unavailable, proceeding to trigger", hostname)
	}

	// latestVersion is used for reporting; fall back to empty if check skipped.
	latestVersion := ""
	if checkResult != nil {
		latestVersion = checkResult.LatestVersion
	}

	// Trigger the update (30s backstop).
	triggerCtx, triggerCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer triggerCancel()
	triggerResult, statusCode, err := client.Trigger(triggerCtx)
	if err != nil {
		return fmt.Errorf("[%s] trigger failed: %w", hostname, err)
	}

	// Handle by trigger status string first (firmware-level semantics), then
	// fall through to status-code handling for the pull path.
	if triggerResult != nil && triggerResult.Status == "rebooting_for_boot_mode_ota" {
		return handleBootModeTrigger(client, hostname, latestVersion)
	}

	// Handle HTTP status codes (pull path and edge cases).
	switch statusCode {
	case 202:
		// pull path (status "update_started") — fall through to the poll loop below
	case 200:
		output.Success("[%s] update already completed (%s)", hostname, latestVersion)
		return nil
	case 409:
		output.Warn("[%s] update already in progress, polling status", hostname)
		// Continue to poll loop
	default:
		if triggerResult != nil && triggerResult.Error != "" {
			return fmt.Errorf("[%s] trigger returned status %d: %s", hostname, statusCode, triggerResult.Error)
		}
		return fmt.Errorf("[%s] trigger returned unexpected status %d", hostname, statusCode)
	}

	return pollPullUpdate(client, hostname, latestVersion)
}

// handleBootModeTrigger handles the boot-mode OTA path. The device reboots
// immediately after acknowledging the trigger; it pulls the firmware at next
// boot. We wait for it to come back up and verify the version.
func handleBootModeTrigger(client *ota.Client, hostname, expected string) error {
	output.Info("[%s] rebooting to apply OTA (boot-mode)…", hostname)

	// Best-effort: poll /api/update/progress briefly for a progress bar.
	// The device may 404 immediately — ignore any error here.
	progressCtx, progressCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer progressCancel()
	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()
	progressDone := false
	for !progressDone {
		select {
		case <-progressCtx.Done():
			progressDone = true
		case <-progressTicker.C:
			status, err := client.PollStatus(progressCtx)
			if err != nil {
				// Device already gone (rebooting) or route not available — stop polling.
				progressDone = true
				continue
			}
			if status.InProgress || status.State != "idle" {
				fmt.Printf("\r[%s] %s: %.0f%%\033[K", hostname, status.State, status.ProgressPct)
			}
			if status.State == "complete" || status.State == "error" {
				progressDone = true
			}
		}
	}
	fmt.Printf("\n")

	// Wait for device to come back and verify booted version.
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer bootCancel()
	booted, err := client.WaitForBoot(bootCtx, 3*time.Second)
	if err != nil || booted == "" {
		output.Success("[%s] boot-mode OTA applied (expected %s; device not yet responding)", hostname, expected)
		return nil
	}
	if expected != "" {
		expNorm := strings.TrimPrefix(expected, "v")
		bootNorm := strings.TrimPrefix(booted, "v")
		if bootNorm != expNorm {
			output.Warn("[%s] boot-mode OTA applied but booted %s (expected %s)", hostname, booted, expected)
			return nil
		}
	}
	output.Success("[%s] boot-mode OTA completed (%s)", hostname, booted)
	return nil
}

// pollPullUpdate polls /api/update/progress for a pull-mode OTA until
// completion, error, or timeout. Network errors are treated as success only
// after progress has been observed (sawProgress guard).
func pollPullUpdate(client *ota.Client, hostname, latestVersion string) error {
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

			// Connection error during poll likely means device restarted after
			// a successful pull OTA — but ONLY if we already saw progress.
			if err != nil {
				if isNetworkError(err) && sawProgress {
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

// reportBootedVersion polls /api/info after an OTA completes so the user
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

	// Check for net.Error anywhere in the error chain (handles wrapped errors).
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check error message strings for common network errors (including EOF
	// from abrupt connection close during device reboot).
	errMsg := strings.ToLower(err.Error())
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"network unreachable",
		"host unreachable",
		"eof",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(errMsg, netErr) {
			return true
		}
	}

	return false
}
