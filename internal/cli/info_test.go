package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
)

func testInfoResponse() *device.InfoResponse {
	return &device.InfoResponse{
		Board:       "tdongle-s3",
		ProjectName: "TaipanMiner",
		Version:     "v1.2.3",
		IDFVersion:  "v5.3.1",
		Cores:       2,
		MAC:         "aa:bb:cc:dd:ee:ff",
		Network:     device.InfoNetwork{SSID: "HomeWifi"},
		TotalHeap:   327680,
		FreeHeap:    200000,
		FlashSize:   4194304,
		ResetReason: "power-on",
		WDTResets:   0,
	}
}

// TestPrintInfo verifies printInfo renders the SSID from Network and omits Worker.
func TestPrintInfo(t *testing.T) {
	info := testInfoResponse()
	out := captureStdout(t, func() {
		printInfo(info)
	})

	assert.Contains(t, out, "Board:")
	assert.Contains(t, out, "tdongle-s3")
	assert.Contains(t, out, "Version:")
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "IDF Version:")
	assert.Contains(t, out, "v5.3.1")
	assert.Contains(t, out, "MAC:")
	assert.Contains(t, out, "aa:bb:cc:dd:ee:ff")
	assert.Contains(t, out, "SSID:")
	assert.Contains(t, out, "HomeWifi")
	assert.Contains(t, out, "Cores:")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "Heap:")
	assert.Contains(t, out, "200000")
	assert.Contains(t, out, "Flash:")
	assert.Contains(t, out, "Reset Reason:")
	assert.Contains(t, out, "power-on")
	assert.Contains(t, out, "WDT Resets:")

	// Worker line must not appear (removed in this PR)
	assert.NotContains(t, out, "Worker:")
}

// TestPrintInfo_NoOptionalFields verifies BootTime and AppSize lines are absent when nil.
func TestPrintInfo_NoOptionalFields(t *testing.T) {
	info := testInfoResponse()
	out := captureStdout(t, func() {
		printInfo(info)
	})

	assert.NotContains(t, out, "Boot Time:")
	assert.NotContains(t, out, "App Size:")
}

// TestPrintInfo_WithOptionalFields verifies BootTime and AppSize lines appear when set.
func TestPrintInfo_WithOptionalFields(t *testing.T) {
	info := testInfoResponse()
	bootTime := int64(1746000000)
	appSize := uint32(1048576)
	info.BootTime = &bootTime
	info.AppSize = &appSize

	out := captureStdout(t, func() {
		printInfo(info)
	})

	assert.Contains(t, out, "Boot Time:")
	assert.Contains(t, out, "App Size:")
	assert.Contains(t, out, "1048576")
}
