package cli

import (
	"bufio"
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
	configCmd.PersistentFlags().StringVar(&profileFlag, "profile", "default", "Configuration profile name")

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

	fmt.Fprint(os.Stderr, "WiFi SSID: ")
	scanner.Scan()
	profile.WifiSSID = strings.TrimSpace(scanner.Text())

	fmt.Fprint(os.Stderr, "WiFi Password: ")
	scanner.Scan()
	profile.WifiPassword = strings.TrimSpace(scanner.Text())

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

	profile, err := cfg.GetProfile(profileFlag)
	if err != nil {
		return err
	}

	value, err := config.Get(profile, key)
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

	profile, err := cfg.GetProfile(profileFlag)
	if err != nil {
		return err
	}

	if err := config.Set(profile, key, value); err != nil {
		return err
	}

	if err := config.Save(path, cfg); err != nil {
		return err
	}

	output.Success("Configuration updated")
	return nil
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

	profile, err := cfg.GetProfile(profileFlag)
	if err != nil {
		return err
	}

	// Marshal profile to YAML
	data, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}

	fmt.Print(string(data))
	return nil
}
