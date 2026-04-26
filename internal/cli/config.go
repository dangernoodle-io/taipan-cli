package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dangernoodle-io/taipan-cli/internal/config"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

var profileFlag string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage TaipanMiner configuration profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	configCmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "Configuration profile name (required for profile-level keys)")

	configCmd.AddCommand(initCmd)
	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(listCmd)

	rootCmd.AddCommand(configCmd)
}

// initCmd creates a new configuration profile interactively
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new configuration profile",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(os.Stdin)

	// Check if config file exists
	var cfg *config.Config
	if _, err := os.Stat(path); err == nil {
		output.Warn("Configuration file already exists at %s", path)
		fmt.Fprint(os.Stderr, "Overwrite? (yes/no): ")
		scanner.Scan()
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if response != "yes" && response != "y" {
			output.Warn("Cancelled")
			return nil
		}

		// Load existing config to preserve other profiles
		cfg, err = config.Load(path)
		if err != nil {
			output.Warn("Could not load existing config, creating fresh: %v", err)
			cfg = &config.Config{
				Profiles: make(map[string]*config.Profile),
			}
		}
	} else {
		cfg = &config.Config{
			Profiles: make(map[string]*config.Profile),
		}
	}

	// Prompt for profile values
	profile := &config.Profile{
		Boards: make(map[string]*config.BoardEntry),
	}

	// Check if global WiFi exists; if so, skip per-profile WiFi prompts
	if cfg.Wifi != nil && cfg.Wifi.SSID != "" {
		output.Info("Using global wifi (%s)", cfg.Wifi.SSID)
		// Leave profile.WifiSSID/WifiPassword empty so resolution falls through to global
	} else {
		// Prompt for global WiFi on first init, or fall back to per-profile if no global exists
		if cfg.Wifi == nil {
			fmt.Fprint(os.Stderr, "WiFi SSID (global): ")
			scanner.Scan()
			ssid := strings.TrimSpace(scanner.Text())
			if ssid != "" {
				if cfg.Wifi == nil {
					cfg.Wifi = &config.Wifi{}
				}
				cfg.Wifi.SSID = ssid

				fmt.Fprint(os.Stderr, "WiFi Password (global): ")
				scanner.Scan()
				cfg.Wifi.Password = strings.TrimSpace(scanner.Text())
			} else {
				// If user doesn't provide global WiFi, prompt per-profile
				fmt.Fprint(os.Stderr, "WiFi SSID: ")
				scanner.Scan()
				profile.WifiSSID = strings.TrimSpace(scanner.Text())

				fmt.Fprint(os.Stderr, "WiFi Password: ")
				scanner.Scan()
				profile.WifiPassword = strings.TrimSpace(scanner.Text())
			}
		} else {
			// cfg.Wifi exists but SSID is empty (incomplete global); prompt per-profile
			fmt.Fprint(os.Stderr, "WiFi SSID: ")
			scanner.Scan()
			profile.WifiSSID = strings.TrimSpace(scanner.Text())

			fmt.Fprint(os.Stderr, "WiFi Password: ")
			scanner.Scan()
			profile.WifiPassword = strings.TrimSpace(scanner.Text())
		}
	}

	fmt.Fprint(os.Stderr, "Pool Host: ")
	scanner.Scan()
	profile.PoolHost = strings.TrimSpace(scanner.Text())

	fmt.Fprint(os.Stderr, "Pool Port (default 3333): ")
	scanner.Scan()
	portStr := strings.TrimSpace(scanner.Text())
	port := uint16(3333)
	if portStr != "" {
		p, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid pool port: %w", err)
		}
		port = uint16(p)
	}
	profile.PoolPort = port

	fmt.Fprint(os.Stderr, "Wallet: ")
	scanner.Scan()
	profile.Wallet = strings.TrimSpace(scanner.Text())

	fmt.Fprint(os.Stderr, "Worker Prefix: ")
	scanner.Scan()
	profile.WorkerPrefix = strings.TrimSpace(scanner.Text())

	// Store the profile
	cfg.Profiles[profileFlag] = profile

	// Save the config
	if err := config.Save(path, cfg); err != nil {
		return err
	}

	output.Success("Profile %q created successfully at %s", profileFlag, path)
	return nil
}

// getCmd retrieves a configuration value
var getCmd = &cobra.Command{
	Use:   "get KEY",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	// For backwards compatibility: if --profile not set and no leading global segment, use "default"
	profile := profileFlag
	if profile == "" && !isGlobalKey(key) {
		profile = "default"
	}

	value, err := config.Get(cfg, profile, key)
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}

// setCmd sets a configuration value
var setCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runSet,
}

func runSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	// For backwards compatibility: if --profile not set and no leading global segment, use "default"
	profile := profileFlag
	if profile == "" && !isGlobalKey(key) {
		profile = "default"
	}

	if err := config.Set(cfg, profile, key, value); err != nil {
		return err
	}

	if err := config.Save(path, cfg); err != nil {
		return err
	}

	output.Success("Configuration updated")
	return nil
}

// isGlobalKey checks if a key should be routed to global config (no profile required)
func isGlobalKey(key string) bool {
	segments := strings.Split(key, ".")
	if len(segments) == 0 {
		return false
	}
	segment := segments[0]
	return segment == "wifi" || segment == "worker" || segment == "pool" || segment == "boards"
}

// listCmd displays the current profile
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values for a profile",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	profile := profileFlag
	if profile == "" {
		profile = "default"
	}

	p, err := cfg.GetProfile(profile)
	if err != nil {
		return err
	}

	// Marshal profile to YAML with 2-space indent
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(p); err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}
	_ = enc.Close()

	fmt.Print(buf.String())
	return nil
}
