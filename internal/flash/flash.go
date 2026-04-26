package flash

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dangernoodle-io/taipan-cli/internal/config"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	espflasher "tinygo.org/x/espflasher/pkg/espflasher"
	"tinygo.org/x/espflasher/pkg/nvs"
	"golang.org/x/term"
)

// FlashOptions contains the inputs for a flash operation
type FlashOptions struct {
	Board        string
	Port         string
	Profile      string // profile name (default: "default")
	FirmwarePath string // path to firmware binary, empty = download latest
	ConfigPath   string // path to config.yml, empty = default
	Force        bool   // skip pre-flash checks
	Host         string // device host for OTA check, empty = serial flash
	WifiSSID     string // override resolved wifi SSID
	WifiPassword string // override resolved wifi password
	SkipConfirm  bool   // skip confirmation prompt (--yes flag)
	Factory      bool   // flash factory image (default); false = OTA app-only
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
	if err != nil && errors.Is(err, os.ErrNotExist) {
		output.Warn("No config found, skipping NVS provisioning (device will enter AP provisioning mode)")
	} else if err != nil {
		return fmt.Errorf("config error: %w", err)
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
	var actualFirmwarePath string
	if opts.FirmwarePath != "" {
		// Read from provided path
		data, err := os.ReadFile(opts.FirmwarePath)
		if err != nil {
			return fmt.Errorf("cannot read firmware from %s: %w", opts.FirmwarePath, err)
		}
		firmwareBin = data
		actualFirmwarePath = opts.FirmwarePath
		output.Info("Loaded firmware from: %s (%d bytes)", opts.FirmwarePath, len(firmwareBin))
	} else {
		// Download latest firmware
		fwType := "factory"
		if !opts.Factory {
			fwType = "OTA"
		}
		output.Info("Downloading latest %s firmware for board: %s", fwType, opts.Board)
		asset, err := DownloadLatestFirmware(opts.Board, opts.Factory)
		if err != nil {
			return fmt.Errorf("cannot download firmware: %w", err)
		}

		data, err := os.ReadFile(asset.Path)
		if err != nil {
			return fmt.Errorf("cannot read downloaded firmware: %w", err)
		}
		firmwareBin = data
		actualFirmwarePath = asset.Path
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

	// Step 7: Detect serial port (needed by precheck for serial chip probe)
	if opts.Port == "" {
		detected, err := DetectPort()
		if err != nil {
			return err
		}
		opts.Port = detected
	}

	// Step 8: Pre-flash checks
	err = Precheck(opts.Board, actualFirmwarePath, opts.Host, opts.Port, opts.Factory, opts.Force)
	if err != nil {
		return err
	}

	// Step 9: Flash via espflasher
	output.Info("Opening serial port: %s", opts.Port)

	// Dispatch chip by board
	chip, err := ChipForBoard(opts.Board)
	if err != nil {
		return err
	}

	flashOpts := espflasher.DefaultOptions()
	flashOpts.ResetMode = espflasher.ResetAuto
	flashOpts.ChipType = chip
	f, err := espflasher.New(opts.Port, flashOpts)
	if err != nil {
		return fmt.Errorf("cannot open serial port %s: %w", opts.Port, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var images []espflasher.ImagePart
	if opts.Factory {
		// Factory image: flash at offset 0x0 (includes bootloader, partition table, etc.)
		if nvsBin != nil {
			output.Info("Flashing factory image + NVS...")
			// NVS overlays the empty NVS in the factory image
			images = append(images, espflasher.ImagePart{Data: firmwareBin, Offset: 0x0})
			images = append(images, espflasher.ImagePart{Data: nvsBin, Offset: 0x9000})
		} else {
			output.Info("Flashing factory image only...")
			images = append(images, espflasher.ImagePart{Data: firmwareBin, Offset: 0x0})
		}
		// Factory image already includes otadata, so skip otadata erase
	} else {
		// OTA app-only: current behavior (otadata erase + app at 0x20000)
		if nvsBin != nil {
			output.Info("Flashing OTA app + NVS...")
			images = append(images, espflasher.ImagePart{Data: nvsBin, Offset: 0x9000})
		} else {
			output.Info("Flashing OTA app only...")
		}
		// Erase otadata so bootloader defaults to ota_0.
		// Use 0xFF (erased flash state) — all-zero otadata looks corrupted to ESP-IDF's bootloader.
		otadata := make([]byte, 0x2000)
		for i := range otadata {
			otadata[i] = 0xFF
		}
		images = append(images, espflasher.ImagePart{Data: otadata, Offset: 0xf000})
		images = append(images, espflasher.ImagePart{Data: firmwareBin, Offset: 0x20000})
	}

	err = f.FlashImages(images, func(current, total int) {
		fmt.Printf("\rFlashing: %d/%d bytes", current, total)
	})
	if err != nil {
		return fmt.Errorf("flash failed: %w", err)
	}

	fmt.Print("\n")
	f.Reset()
	output.Success("Flash complete!")

	return nil
}

// resolveAndBuildNVS loads the profile, resolves board config, prompts for missing values,
// and generates the NVS binary.
func resolveAndBuildNVS(cfg *config.Config, opts *FlashOptions) ([]byte, error) {
	profile := opts.Profile
	if profile == "" {
		profile = "default"
	}

	resolved, err := config.ResolveBoard(cfg, profile, opts.Board)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve board %q: %w", opts.Board, err)
	}

	// Apply CLI overrides for wifi if provided
	if opts.WifiSSID != "" {
		resolved.WifiSSID = opts.WifiSSID
	}
	if opts.WifiPassword != "" {
		resolved.WifiPassword = opts.WifiPassword
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
		// Assemble worker from resolved prefix + suffix
		prefix := resolved.WorkerPrefix
		if prefix != "" {
			resolved.Worker = prefix + "-" + suffix
		} else {
			resolved.Worker = suffix
		}
	}

	// Confirm resolved configuration
	if err := confirmResolved(resolved, opts.Board, profile, opts.SkipConfirm); err != nil {
		return nil, err
	}

	displayEnValue := uint8(0)
	if resolved.DisplayEn {
		displayEnValue = 1
	}

	entries := []nvs.Entry{
		{Namespace: "taipanminer", Key: "wifi_ssid", Type: "string", Value: resolved.WifiSSID},
		{Namespace: "taipanminer", Key: "wifi_pass", Type: "string", Value: resolved.WifiPassword},
		{Namespace: "taipanminer", Key: "pool_host", Type: "string", Value: resolved.PoolHost},
		{Namespace: "taipanminer", Key: "pool_port", Type: "u16", Value: resolved.PoolPort},
		{Namespace: "taipanminer", Key: "pool_pass", Type: "string", Value: resolved.PoolPassword},
		{Namespace: "taipanminer", Key: "wallet_addr", Type: "string", Value: resolved.Wallet},
		{Namespace: "taipanminer", Key: "worker", Type: "string", Value: resolved.Worker},
		{Namespace: "taipanminer", Key: "display_en", Type: "u8", Value: displayEnValue},
		{Namespace: "taipanminer", Key: "provisioned", Type: "u8", Value: uint8(1)},
	}

	return nvs.GenerateNVS(entries, nvs.DefaultPartSize)
}

// confirmResolved prints the resolved NVS configuration and prompts for confirmation.
// If skip is true, returns nil immediately.
// If stdin is not a TTY, returns nil (non-interactive, suitable for CI).
// Otherwise, prompts on stderr for confirmation. Empty/y/Y/yes proceeds; anything else aborts.
func confirmResolved(resolved *config.ResolvedConfig, board, profile string, skip bool) error {
	if skip {
		return nil
	}

	// Check if stdin is a TTY
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}

	// Print resolved config table to stderr
	fmt.Fprintf(os.Stderr, "\nResolved NVS config for %s (profile=%s):\n", board, profile)
	fmt.Fprintf(os.Stderr, "  wifi.ssid       %s\n", resolved.WifiSSID)
	fmt.Fprintf(os.Stderr, "  wifi.password   ********\n") // masked
	fmt.Fprintf(os.Stderr, "  pool.host       %s\n", resolved.PoolHost)
	fmt.Fprintf(os.Stderr, "  pool.port       %d\n", resolved.PoolPort)
	fmt.Fprintf(os.Stderr, "  pool.password   ********\n") // masked

	// Truncate wallet to first 13 chars + "..."
	wallet := resolved.Wallet
	if len(wallet) > 13 {
		wallet = wallet[:13] + "..."
	}
	fmt.Fprintf(os.Stderr, "  pool.wallet     %s\n", wallet)
	fmt.Fprintf(os.Stderr, "  worker          %s\n", resolved.Worker)
	displayEnStr := "false"
	if resolved.DisplayEn {
		displayEnStr = "true"
	}
	fmt.Fprintf(os.Stderr, "  display_en      %s\n", displayEnStr)

	// Prompt for confirmation
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "\nProceed? [Y/n]: ")
	scanner.Scan()
	response := strings.ToLower(strings.TrimSpace(scanner.Text()))

	// Empty, y, Y, or yes means proceed
	if response == "" || response == "y" || response == "yes" {
		return nil
	}

	return errors.New("aborted by user")
}

// promptValue reads a single line from stdin, returning the trimmed input
func promptValue(scanner *bufio.Scanner, label string) string {
	fmt.Printf("%s: ", label)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}
