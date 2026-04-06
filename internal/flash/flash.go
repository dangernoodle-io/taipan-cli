package flash

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dangernoodle-io/taipan-cli/internal/config"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	espflasher "tinygo.org/x/espflasher/pkg/espflasher"
)

// FlashOptions contains the inputs for a flash operation
type FlashOptions struct {
	Board        string
	Port         string
	Profile      string // profile name (default: "default")
	FirmwarePath string // path to firmware binary, empty = download latest
	ConfigPath   string // path to config.yml, empty = default
}

// Flash performs the full flash operation: config resolution → interactive prompting →
// NVS generation → firmware download → serial flash.
func Flash(opts *FlashOptions) error {
	// Step 1: Load config and resolve NVS (skip if no config exists)
	var nvsBin []byte

	configPath := opts.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = config.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine default config path: %w", err)
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		output.Warn("No config found, skipping NVS provisioning (device will enter AP provisioning mode)")
	} else {
		nvsBin, err = resolveAndBuildNVS(cfg, opts)
		if err != nil {
			return err
		}
		output.Info("Generated NVS partition: %d bytes", len(nvsBin))
	}

	// Step 6: Get firmware binary
	var firmwareBin []byte
	var cleanupPath string // Track parent temp directory for cleanup
	if opts.FirmwarePath != "" {
		// Read from provided path
		data, err := os.ReadFile(opts.FirmwarePath)
		if err != nil {
			return fmt.Errorf("cannot read firmware from %s: %w", opts.FirmwarePath, err)
		}
		firmwareBin = data
		output.Info("Loaded firmware from: %s (%d bytes)", opts.FirmwarePath, len(firmwareBin))
	} else {
		// Download latest firmware
		output.Info("Downloading latest firmware for board: %s", opts.Board)
		asset, err := DownloadLatestFirmware(opts.Board)
		if err != nil {
			return fmt.Errorf("cannot download firmware: %w", err)
		}

		data, err := os.ReadFile(asset.Path)
		if err != nil {
			return fmt.Errorf("cannot read downloaded firmware: %w", err)
		}
		firmwareBin = data
		output.Success("Downloaded %s (%s, %d bytes)", asset.Name, asset.Version, len(firmwareBin))

		// Store parent directory (where temp dir was created) for cleanup
		cleanupPath = asset.Path
		defer func() {
			if cleanupPath != "" {
				// Clean up the temp directory that contains the firmware file
				_ = os.RemoveAll(filepath.Dir(cleanupPath))
			}
		}()
	}

	// Step 7: Flash via espflasher
	if opts.Port == "" {
		detected, err := DetectPort()
		if err != nil {
			return err
		}
		opts.Port = detected
	}
	output.Info("Opening serial port: %s", opts.Port)
	flashOpts := espflasher.DefaultOptions()
	flashOpts.ResetMode = espflasher.ResetNoReset
	flashOpts.ChipType = espflasher.ChipESP32S3
	f, err := espflasher.New(opts.Port, flashOpts)
	if err != nil {
		if strings.Contains(err.Error(), "failed to sync") {
			return fmt.Errorf("device not in download mode — unplug, hold the BOOT button, plug back in, then release BOOT and retry")
		}
		return fmt.Errorf("cannot open serial port %s: %w", opts.Port, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var images []espflasher.ImagePart
	if nvsBin != nil {
		output.Info("Flashing NVS and firmware...")
		images = append(images, espflasher.ImagePart{Data: nvsBin, Offset: 0x9000})
	} else {
		output.Info("Flashing firmware only...")
	}
	images = append(images, espflasher.ImagePart{Data: firmwareBin, Offset: 0x20000})

	err = f.FlashImages(images, func(current, total int) {
		fmt.Printf("\rFlashing: %d/%d bytes", current, total)
	})
	if err != nil {
		return fmt.Errorf("flash failed: %w", err)
	}

	fmt.Print("\n")
	f.Reset()
	output.Success("Flash complete!")
	output.Warn("USB CDC does not support hardware reset — unplug and replug the device to boot")

	return nil
}

// resolveAndBuildNVS loads the profile, resolves board config, prompts for missing values,
// and generates the NVS binary.
func resolveAndBuildNVS(cfg *config.Config, opts *FlashOptions) ([]byte, error) {
	profile := opts.Profile
	if profile == "" {
		profile = "default"
	}

	profileCfg, err := cfg.GetProfile(profile)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile %q: %w", profile, err)
	}

	resolved, err := config.ResolveBoard(profileCfg, opts.Board)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve board %q: %w", opts.Board, err)
	}

	scanner := bufio.NewScanner(os.Stdin)

	if resolved.WifiSSID == "" {
		resolved.WifiSSID = promptValue(scanner, "WiFi SSID")
		if resolved.WifiSSID == "" {
			return nil, fmt.Errorf("WiFi SSID cannot be empty")
		}
	}

	if resolved.WifiPassword == "" {
		resolved.WifiPassword = promptValue(scanner, "WiFi Password")
		if resolved.WifiPassword == "" {
			return nil, fmt.Errorf("WiFi Password cannot be empty")
		}
	}

	if resolved.PoolHost == "" {
		resolved.PoolHost = promptValue(scanner, "Pool Host")
		if resolved.PoolHost == "" {
			return nil, fmt.Errorf("pool host cannot be empty")
		}
	}

	if resolved.PoolPort == 0 {
		portStr := promptValue(scanner, "Pool Port")
		if portStr == "" {
			return nil, fmt.Errorf("pool port cannot be empty")
		}
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid pool port: %w", err)
		}
		resolved.PoolPort = uint16(port)
	}

	if resolved.Wallet == "" {
		resolved.Wallet = promptValue(scanner, "Wallet Address")
		if resolved.Wallet == "" {
			return nil, fmt.Errorf("wallet address cannot be empty")
		}
	}

	if resolved.Worker == "" {
		suffix := promptValue(scanner, "Worker Suffix")
		if suffix == "" {
			return nil, fmt.Errorf("worker suffix cannot be empty")
		}
		resolved.Worker = profileCfg.WorkerPrefix + "-" + suffix
	}

	entries := []NVSEntry{
		{Namespace: "taipanminer", Key: "wifi_ssid", Type: "string", Value: resolved.WifiSSID},
		{Namespace: "taipanminer", Key: "wifi_pass", Type: "string", Value: resolved.WifiPassword},
		{Namespace: "taipanminer", Key: "pool_host", Type: "string", Value: resolved.PoolHost},
		{Namespace: "taipanminer", Key: "pool_port", Type: "u16", Value: resolved.PoolPort},
		{Namespace: "taipanminer", Key: "wallet_addr", Type: "string", Value: resolved.Wallet},
		{Namespace: "taipanminer", Key: "worker", Type: "string", Value: resolved.Worker},
		{Namespace: "taipanminer", Key: "pool_pass", Type: "string", Value: "x"},
		{Namespace: "taipanminer", Key: "provisioned", Type: "u8", Value: uint8(1)},
	}

	return GenerateNVS(entries, 0x6000)
}

// promptValue reads a single line from stdin, returning the trimmed input
func promptValue(scanner *bufio.Scanner, label string) string {
	fmt.Printf("%s: ", label)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}
