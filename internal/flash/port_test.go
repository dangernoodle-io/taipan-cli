package flash

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsUSBSerial(t *testing.T) {
	tests := []struct {
		name string
		port string
		goos string
		want bool
	}{
		{"darwin usbmodem", "/dev/cu.usbmodem101", "darwin", true},
		{"darwin usbserial", "/dev/cu.usbserial-110", "darwin", true},
		{"darwin bluetooth", "/dev/cu.Bluetooth-Incoming-Port", "darwin", false},
		{"darwin tty", "/dev/tty.usbmodem101", "darwin", false},
		{"linux ttyUSB", "/dev/ttyUSB0", "linux", true},
		{"linux ttyACM", "/dev/ttyACM0", "linux", true},
		{"linux ttyS", "/dev/ttyS0", "linux", false},
		{"windows COM", "COM3", "windows", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS != tt.goos {
				t.Skipf("test only runs on %s", tt.goos)
			}
			assert.Equal(t, tt.want, isUSBSerial(tt.port))
		})
	}
}
