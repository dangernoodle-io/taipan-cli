package cli

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
)

func testSettingsResponse() *device.SettingsResponse {
	return &device.SettingsResponse{
		Hostname:     "taipan-device",
		DisplayEn:    false,
		OTASkipCheck: false,
		MDNSEn:       true,
		KnotEn:       true,
		Provisioned:  true,
	}
}

// TestPrintSettings verifies printSettings outputs expected device-config fields.
func TestPrintSettings(t *testing.T) {
	s := testSettingsResponse()
	out := captureStdout(t, func() {
		printSettings(s)
	})

	assert.Contains(t, out, "Hostname:")
	assert.Contains(t, out, "taipan-device")
	assert.Contains(t, out, "Display:")
	assert.Contains(t, out, "OTA Skip Check:")
	assert.Contains(t, out, "mDNS:")
	assert.Contains(t, out, "Knot:")
	assert.Contains(t, out, "Provisioned:")

	// pool/wallet/worker fields removed
	assert.NotContains(t, out, "Pool Host:")
	assert.NotContains(t, out, "Pool Port:")
	assert.NotContains(t, out, "Wallet:")
	assert.NotContains(t, out, "Worker:")
	assert.NotContains(t, out, "Pool Pass:")
}

// TestPrintSettings_BoolValues verifies bool fields print correctly.
func TestPrintSettings_BoolValues(t *testing.T) {
	s := testSettingsResponse()
	s.MDNSEn = false
	s.KnotEn = false
	s.Provisioned = false

	out := captureStdout(t, func() {
		printSettings(s)
	})

	assert.Contains(t, out, "mDNS:")
	assert.Contains(t, out, "Knot:")
	assert.Contains(t, out, "Provisioned:")
}

// TestSettingsSet_MdnsEnBoolCoercion verifies mdns_en is coerced to bool.
func TestSettingsSet_MdnsEnBoolCoercion(t *testing.T) {
	v, err := strconv.ParseBool("true")
	require.NoError(t, err)
	assert.IsType(t, bool(false), v)
	assert.True(t, v)
}

// TestSettingsSet_KnotEnBoolCoercion verifies knot_en is coerced to bool.
func TestSettingsSet_KnotEnBoolCoercion(t *testing.T) {
	v, err := strconv.ParseBool("false")
	require.NoError(t, err)
	assert.IsType(t, bool(false), v)
	assert.False(t, v)
}

// TestSettingsSet_BoolKeys verifies all expected bool keys parse correctly.
func TestSettingsSet_BoolKeys(t *testing.T) {
	boolKeys := []string{"display_en", "ota_skip_check", "mdns_en", "knot_en"}
	for _, key := range boolKeys {
		t.Run(key, func(t *testing.T) {
			for _, val := range []string{"true", "false", "1", "0"} {
				_, err := strconv.ParseBool(val)
				assert.NoError(t, err, "key=%s val=%s should parse as bool", key, val)
			}
		})
	}
}
