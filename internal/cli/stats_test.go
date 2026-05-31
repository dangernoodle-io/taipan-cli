package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
)

func testStatsResponse() *device.StatsResponse {
	return &device.StatsResponse{
		Hashrate:        1.5e9,
		HashrateAvg:     1.4e9,
		TempC:           45.5,
		Shares:          100,
		SessionShares:   50,
		SessionRejected: 2,
		LastShareAgoS:   30,
		BestDiff:        2.5,
		UptimeS:         3600,
	}
}

// TestPrintStats verifies printStats outputs expected mining lines.
func TestPrintStats(t *testing.T) {
	s := testStatsResponse()
	out := captureStdout(t, func() {
		printStats(s)
	})

	assert.Contains(t, out, "Hashrate:")
	assert.Contains(t, out, "1.50 GH/s")
	assert.Contains(t, out, "Avg Hashrate:")
	assert.Contains(t, out, "1.40 GH/s")
	assert.Contains(t, out, "Temp:")
	assert.Contains(t, out, "45.5")
	assert.Contains(t, out, "Shares:")
	assert.Contains(t, out, "50 accepted / 2 rejected")
	assert.Contains(t, out, "Best Diff:")
	assert.Contains(t, out, "2.5000")
	assert.Contains(t, out, "Uptime:")
	assert.Contains(t, out, "1h")
	assert.Contains(t, out, "Last Share:")
	assert.Contains(t, out, "30s ago")

	// pool/worker/lifetime removed
	assert.NotContains(t, out, "Pool:")
	assert.NotContains(t, out, "Worker:")
	assert.NotContains(t, out, "Pool Diff:")
	assert.NotContains(t, out, "Lifetime:")
}

// TestPrintStats_NoBestDiff verifies the "--" fallback when BestDiff is zero.
func TestPrintStats_NoBestDiff(t *testing.T) {
	s := testStatsResponse()
	s.BestDiff = 0
	out := captureStdout(t, func() {
		printStats(s)
	})
	assert.Contains(t, out, "Best Diff:")
	assert.Contains(t, out, "--")
}

// TestPrintStats_ASICFields verifies that asic_* lines appear when set.
func TestPrintStats_ASICFields(t *testing.T) {
	asicHashrate := 5.0e9
	asicHashrateAvg := 4.9e9
	asicTempC := 65.0
	asicShares := uint32(45)

	s := testStatsResponse()
	s.AsicHashrate = &asicHashrate
	s.AsicHashrateAvg = &asicHashrateAvg
	s.AsicTempC = &asicTempC
	s.AsicShares = &asicShares

	out := captureStdout(t, func() {
		printStats(s)
	})

	assert.Contains(t, out, "ASIC Hashrate:")
	assert.Contains(t, out, "5.00 GH/s")
	assert.Contains(t, out, "ASIC Avg:")
	assert.Contains(t, out, "4.90 GH/s")
	assert.Contains(t, out, "ASIC Temp:")
	assert.Contains(t, out, "65.0")
	assert.Contains(t, out, "ASIC Shares:")
	assert.Contains(t, out, "45")
}

// TestPrintStats_NoASICFields verifies asic lines absent when nil.
func TestPrintStats_NoASICFields(t *testing.T) {
	s := testStatsResponse()
	out := captureStdout(t, func() {
		printStats(s)
	})
	assert.NotContains(t, out, "ASIC Hashrate:")
	assert.NotContains(t, out, "ASIC Avg:")
	assert.NotContains(t, out, "ASIC Temp:")
	assert.NotContains(t, out, "ASIC Shares:")
}

// TestFmtHashrate covers all magnitude branches.
func TestFmtHashrate(t *testing.T) {
	assert.Equal(t, "1.00 TH/s", fmtHashrate(1e12))
	assert.Equal(t, "1.50 GH/s", fmtHashrate(1.5e9))
	assert.Equal(t, "2.00 MH/s", fmtHashrate(2e6))
	assert.Equal(t, "1.00 kH/s", fmtHashrate(1e3))
	assert.Equal(t, "500.00 H/s", fmtHashrate(500))
}

// TestFmtDiff covers all magnitude branches.
func TestFmtDiff(t *testing.T) {
	assert.Equal(t, "1.00T", fmtDiff(1e12))
	assert.Equal(t, "2.00G", fmtDiff(2e9))
	assert.Equal(t, "3.00M", fmtDiff(3e6))
	assert.Equal(t, "4.00k", fmtDiff(4e3))
	assert.Equal(t, "0.0625", fmtDiff(0.0625))
}

// TestFmtUptime covers hours, minutes-only, and seconds-only branches.
func TestFmtUptime(t *testing.T) {
	assert.Contains(t, fmtUptime(3661), "1h")
	assert.Contains(t, fmtUptime(90), "1m")
	assert.Equal(t, "45s", fmtUptime(45))
}

// TestFmtLastShare covers all branches including "never".
func TestFmtLastShare(t *testing.T) {
	assert.Equal(t, "never", fmtLastShare(-1))
	assert.Contains(t, fmtLastShare(3661), "1h")
	assert.Contains(t, fmtLastShare(90), "1m")
	assert.Contains(t, fmtLastShare(5), "5s ago")
}
