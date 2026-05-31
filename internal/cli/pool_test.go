package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
)

func testPoolResponse() *device.PoolResponse {
	latency := 40
	versionMask := "1fffe000"
	sessionAgo := int64(48271)
	return &device.PoolResponse{
		Host:                  "hmpool.io",
		Port:                  3337,
		Worker:                "tdongleS3-3",
		Wallet:                "bc1q-test-wallet",
		Connected:             true,
		SessionStartAgoS:      &sessionAgo,
		CurrentDifficulty:     0.01,
		PoolEffectiveHashrate: 311378,
		LatencyMs:             &latency,
		VersionMask:           &versionMask,
		ActivePoolIdx:         0,
		LifetimeBlocksTotal:   1,
		Stats: []device.PoolStat{
			{
				Host:        "hmpool.io",
				Port:        3337,
				Shares:      1952,
				BestDiff:    27.9872,
				BlocksFound: 0,
				LastSeenS:   4,
			},
		},
	}
}

// TestPrintPool verifies that printPool outputs expected lines.
func TestPrintPool(t *testing.T) {
	p := testPoolResponse()
	out := captureStdout(t, func() {
		printPool(p)
	})

	assert.Contains(t, out, "Pool:")
	assert.Contains(t, out, "hmpool.io:3337")
	assert.Contains(t, out, "Connected:")
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "Worker:")
	assert.Contains(t, out, "tdongleS3-3")
	assert.Contains(t, out, "Wallet:")
	assert.Contains(t, out, "bc1q-test-wallet")
	assert.Contains(t, out, "Difficulty:")
	assert.Contains(t, out, "Pool Hashrate:")
	assert.Contains(t, out, "Best Diff:")
	assert.Contains(t, out, "27.9872")
	assert.Contains(t, out, "Blocks Found:")
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "Latency:")
	assert.Contains(t, out, "40 ms")
	assert.Contains(t, out, "Session:")
	assert.Contains(t, out, "ago")
}

// TestPrintPool_NoOptionalFields verifies printPool handles nil optional fields.
func TestPrintPool_NoOptionalFields(t *testing.T) {
	p := &device.PoolResponse{
		Host:                  "pool.example.com",
		Port:                  3333,
		Worker:                "test-worker",
		Wallet:                "test-wallet",
		Connected:             false,
		CurrentDifficulty:     0.0625,
		PoolEffectiveHashrate: 1e6,
		LifetimeBlocksTotal:   0,
	}

	out := captureStdout(t, func() {
		printPool(p)
	})

	assert.Contains(t, out, "pool.example.com:3333")
	assert.Contains(t, out, "false")
	// no latency or session lines when nil
	assert.NotContains(t, out, "Latency:")
	assert.NotContains(t, out, "Session:")
	// no best diff when stats empty
	assert.NotContains(t, out, "Best Diff:")
}

// TestPrintPool_JSON verifies the --json path marshals the struct correctly.
func TestPrintPool_JSON(t *testing.T) {
	p := testPoolResponse()
	data, err := json.MarshalIndent(p, "", "  ")
	require.NoError(t, err)

	jsonStr := string(data)
	assert.True(t, strings.Contains(jsonStr, `"host": "hmpool.io"`))
	assert.True(t, strings.Contains(jsonStr, `"port": 3337`))
	assert.True(t, strings.Contains(jsonStr, `"worker": "tdongleS3-3"`))
	assert.True(t, strings.Contains(jsonStr, `"current_difficulty": 0.01`))
	assert.True(t, strings.Contains(jsonStr, `"lifetime_blocks_total": 1`))
}
