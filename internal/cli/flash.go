package cli

import (
	"github.com/spf13/cobra"

	"github.com/dangernoodle-io/taipan-cli/internal/flash"
)

var (
	flashBoard       string
	flashPort        string
	flashProfile     string
	flashLatest      bool
	flashForce       bool
	flashWifiSSID    string
	flashWifiPassword string
	flashYes         bool
	flashOTA         bool
)

var flashCmd = &cobra.Command{
	Use:   "flash [firmware.bin]",
	Short: "Flash firmware and configuration to a TaipanMiner device",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFlash,
}

func init() {
	flashCmd.Flags().StringVarP(&flashBoard, "board", "b", "", "Board type (required)")
	flashCmd.Flags().StringVarP(&flashPort, "port", "p", "", "Serial port")
	flashCmd.Flags().StringVar(&flashProfile, "profile", "default", "Config profile")
	flashCmd.Flags().BoolVar(&flashLatest, "latest", false, "Pull latest release from GitHub")
	flashCmd.Flags().BoolVar(&flashForce, "force", false, "Skip pre-flash checks")
	flashCmd.Flags().StringVar(&flashWifiSSID, "wifi-ssid", "", "Override WiFi SSID")
	flashCmd.Flags().StringVar(&flashWifiPassword, "wifi-password", "", "Override WiFi password")
	flashCmd.Flags().BoolVarP(&flashYes, "yes", "y", false, "Skip confirmation prompt")
	flashCmd.Flags().BoolVar(&flashOTA, "ota", false, "Flash OTA app-only image (default: factory image)")
	_ = flashCmd.MarkFlagRequired("board")

	rootCmd.AddCommand(flashCmd)
}

func runFlash(cmd *cobra.Command, args []string) error {
	// Determine firmware path from args (first arg if provided, empty otherwise)
	var firmwarePath string
	if len(args) > 0 {
		firmwarePath = args[0]
	}

	// Call flash.Flash with the collected flags
	opts := &flash.FlashOptions{
		Board:        flashBoard,
		Port:         flashPort,
		Profile:      flashProfile,
		FirmwarePath: firmwarePath,
		Force:        flashForce,
		WifiSSID:     flashWifiSSID,
		WifiPassword: flashWifiPassword,
		SkipConfirm:  flashYes,
		Factory:      !flashOTA,
	}

	return flash.Flash(opts)
}
