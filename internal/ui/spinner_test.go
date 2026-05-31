package ui

import (
	"os"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInactiveWhenDisabled(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
	}()

	SetEnabled(false)
	isTTY = func() bool { return true }

	assert.False(t, active())
}

func TestInactiveWhenNotTTY(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
	}()

	SetEnabled(true)
	isTTY = func() bool { return false }

	assert.False(t, active())
}

func TestNoOpWhenInactive(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
	}()

	SetEnabled(false)
	isTTY = func() bool { return false }

	// Capture stderr to confirm nothing is written.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	stop := Single("msg")
	stop()

	require.NoError(t, w.Close())

	var buf [1]byte
	n, _ := r.Read(buf[:])
	assert.Equal(t, 0, n, "expected no output to stderr when inactive")
	require.NoError(t, r.Close())
}

func TestSingleActive(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
	}()

	SetEnabled(true)
	isTTY = func() bool { return true }

	stop := Single("working...")
	require.NotNil(t, stop)
	stop()
}

func TestSingleRestoresColorNoColor(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	origNoColor := color.NoColor
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
		color.NoColor = origNoColor
	}()

	SetEnabled(true)
	isTTY = func() bool { return true }

	// Set a known value before calling Single
	color.NoColor = true
	stop := Single("working...")
	// After stop, it should be restored
	require.NotNil(t, stop)
	stop()
	assert.Equal(t, true, color.NoColor, "color.NoColor should be restored to original value")
}

func TestSingleDisabledLeavesColorNoColorUntouched(t *testing.T) {
	origEnabled := enabled
	origTTY := isTTY
	origNoColor := color.NoColor
	defer func() {
		enabled = origEnabled
		isTTY = origTTY
		color.NoColor = origNoColor
	}()

	SetEnabled(false)
	isTTY = func() bool { return true }

	color.NoColor = false
	stop := Single("working...")
	stop()
	assert.Equal(t, false, color.NoColor, "color.NoColor should not change when Single is disabled")
}
