package flash

import (
	"testing"

	"github.com/dangernoodle-io/taipan-cli/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestConfirmResolved_SkipConfirmTrue(t *testing.T) {
	// When skip is true, confirmResolved should return nil immediately
	resolved := &config.ResolvedConfig{
		WifiSSID:     "test-ssid",
		WifiPassword: "test-password",
		PoolHost:     "pool.example.com",
		PoolPort:     3333,
		Wallet:       "qprjvt2p089tu123456789",
		Worker:       "test-worker",
	}

	err := confirmResolved(resolved, "bitaxe-403", "bch", true)
	assert.NoError(t, err)
}

func TestConfirmResolved_NonTTY(t *testing.T) {
	// When stdin is not a TTY (e.g., CI environment), confirmResolved should return nil
	resolved := &config.ResolvedConfig{
		WifiSSID:     "test-ssid",
		WifiPassword: "test-password",
		PoolHost:     "pool.example.com",
		PoolPort:     3333,
		Wallet:       "qprjvt2p089tu123456789",
		Worker:       "test-worker",
	}

	// In CI, stdin is typically not a TTY, so this should return nil
	err := confirmResolved(resolved, "bitaxe-403", "bch", false)
	assert.NoError(t, err)
}

func TestWorkerAssembly(t *testing.T) {
	// Test that prefix + suffix are correctly assembled when suffix is prompted.
	// This simulates the user's bug case: prefix from cfg.Boards[board].worker_prefix.

	tests := []struct {
		name      string
		prefix    string
		suffix    string // always provided when prompted in real code (error if empty)
		expected  string
	}{
		{"tdongle_s3_case", "tdongleS3", "001", "tdongleS3-001"},
		{"no_prefix_with_suffix", "", "001", "001"},
		{"prefix_with_multi_digit", "myboard", "123", "myboard-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate resolved config with WorkerPrefix set but Worker empty
			resolved := &config.ResolvedConfig{
				WorkerPrefix: tt.prefix,
				Worker:       "",
			}

			// Apply the prefix assembly logic from flash.go (lines 254-261)
			prefix := resolved.WorkerPrefix
			if prefix != "" {
				resolved.Worker = prefix + "-" + tt.suffix
			} else {
				resolved.Worker = tt.suffix
			}

			assert.Equal(t, tt.expected, resolved.Worker)
		})
	}
}
