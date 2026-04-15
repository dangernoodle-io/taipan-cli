package flash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	espflasher "tinygo.org/x/espflasher/pkg/espflasher"

	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

const (
	// ota0SlotSize is the size of the ota_0 partition in bytes (1.875 MB)
	ota0SlotSize = 1966080
)

// Precheck validates firmware against board and device configuration.
// If force is true, all checks are skipped with a single warning.
func Precheck(board, binPath, host, port string, force bool) error {
	if force {
		output.Warn("Skipping pre-flash checks (--force)")
		return nil
	}

	// Step 1: Parse app descriptor and verify project name contains board
	firmwareInfo, err := ParseFirmwareInfo(binPath)
	if err != nil {
		return fmt.Errorf("cannot parse firmware: %w", err)
	}

	if !strings.Contains(firmwareInfo.ProjectName, board) {
		return fmt.Errorf("firmware project name %q does not contain board %q",
			firmwareInfo.ProjectName, board)
	}

	// Step 2: Validate binary size fits in ota_0 slot
	stat, err := os.Stat(binPath)
	if err != nil {
		return fmt.Errorf("cannot stat firmware: %w", err)
	}
	if stat.Size() > int64(ota0SlotSize) {
		return fmt.Errorf("firmware size %d exceeds ota_0 slot size %d",
			stat.Size(), ota0SlotSize)
	}

	// Step 3: Device cross-check
	if host != "" {
		// OTA path: HTTP GET /api/info and check board field
		if err := checkDeviceViaHTTP(host, board); err != nil {
			return err
		}
	} else {
		// Serial path: probe chip and compare to board
		chip, err := ChipForBoard(board)
		if err != nil {
			return err
		}
		if err := checkDeviceViaProbe(port, chip); err != nil {
			return err
		}
	}

	return nil
}

// checkDeviceViaHTTP performs an OTA-path device cross-check via HTTP /api/info
func checkDeviceViaHTTP(host, expectedBoard string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	url := fmt.Sprintf("http://%s/api/info", host)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("cannot connect to device at %s: %w", host, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("device /api/info returned %d", resp.StatusCode)
	}

	var info struct {
		Board string `json:"board"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("cannot parse device info: %w", err)
	}

	if info.Board != expectedBoard {
		return fmt.Errorf("device board %q does not match --board %q",
			info.Board, expectedBoard)
	}

	return nil
}

// checkDeviceViaProbe performs a serial-path device cross-check by opening the
// port, letting espflasher auto-detect the chip, and comparing to the expected
// ChipType for the target board.
func checkDeviceViaProbe(port string, expected espflasher.ChipType) error {
	opts := espflasher.DefaultOptions()
	opts.ChipType = espflasher.ChipAuto
	opts.ResetMode = espflasher.ResetAuto
	f, err := espflasher.New(port, opts)
	if err != nil {
		return fmt.Errorf("cannot probe chip on %s: %w", port, err)
	}
	defer func() {
		_ = f.Close()
	}()
	actual := f.ChipType()
	if actual != expected {
		return fmt.Errorf("device chip %s does not match board expected %s", actual, expected)
	}
	return nil
}
