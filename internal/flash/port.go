package flash

import (
	"fmt"
	"runtime"
	"strings"

	"go.bug.st/serial"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
)

// DetectPort finds a single USB serial port. Returns an error if zero or multiple ports are found.
func DetectPort() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("cannot list serial ports: %w", err)
	}

	var usbPorts []string
	for _, p := range ports {
		if isUSBSerial(p) {
			usbPorts = append(usbPorts, p)
		}
	}

	switch len(usbPorts) {
	case 0:
		return "", fmt.Errorf("no USB serial ports detected — connect your device and retry")
	case 1:
		output.Info("Auto-detected serial port: %s", usbPorts[0])
		return usbPorts[0], nil
	default:
		return "", fmt.Errorf("multiple USB serial ports detected, specify one with --port:\n  %s", strings.Join(usbPorts, "\n  "))
	}
}

// isUSBSerial returns true if the port name looks like a USB serial device.
func isUSBSerial(port string) bool {
	switch runtime.GOOS {
	case "darwin":
		// macOS: /dev/cu.usbmodem* (CDC) or /dev/cu.usbserial* (FTDI/CH340)
		// Skip /dev/tty.* (blocking dial-in ports) and Bluetooth
		return strings.HasPrefix(port, "/dev/cu.usbmodem") || strings.HasPrefix(port, "/dev/cu.usbserial")
	case "linux":
		// Linux: /dev/ttyUSB* (FTDI/CH340) or /dev/ttyACM* (CDC)
		return strings.HasPrefix(port, "/dev/ttyUSB") || strings.HasPrefix(port, "/dev/ttyACM")
	case "windows":
		// Windows: COM ports (all are potentially USB serial)
		return strings.HasPrefix(port, "COM")
	default:
		return true
	}
}
