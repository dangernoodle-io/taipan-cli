package ui

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInactiveWhenDisabled(t *testing.T) {
	orig := enabled
	origTTY := isTTY
	defer func() {
		enabled = orig
		isTTY = origTTY
	}()

	SetEnabled(false)
	isTTY = func() bool { return true }

	g := NewGroup()
	assert.False(t, g.on)
}

func TestInactiveWhenNotTTY(t *testing.T) {
	orig := enabled
	origTTY := isTTY
	defer func() {
		enabled = orig
		isTTY = origTTY
	}()

	SetEnabled(true)
	isTTY = func() bool { return false }

	g := NewGroup()
	assert.False(t, g.on)
}

func TestNoOpWhenInactive(t *testing.T) {
	orig := enabled
	origTTY := isTTY
	defer func() {
		enabled = orig
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

	g := NewGroup()
	l := g.Add("test")
	g.Start()
	l.Update("updated")
	l.Complete("done")
	l.Error("oops")
	g.Stop()

	// Single convenience function.
	stop := Single("msg")
	stop()

	require.NoError(t, w.Close())

	var buf [1]byte
	n, _ := r.Read(buf[:])
	assert.Equal(t, 0, n, "expected no output to stderr when inactive")
	require.NoError(t, r.Close())
}

func TestSingleActive(t *testing.T) {
	orig := enabled
	origTTY := isTTY
	defer func() {
		enabled = orig
		isTTY = origTTY
	}()

	SetEnabled(true)
	isTTY = func() bool { return true }

	stop := Single("working...")
	stop()
}

func TestActiveGroup(t *testing.T) {
	orig := enabled
	origTTY := isTTY
	defer func() {
		enabled = orig
		isTTY = origTTY
	}()

	SetEnabled(true)
	isTTY = func() bool { return true }

	g := NewGroup()
	assert.True(t, g.on)
	require.NotNil(t, g.sm)

	l := g.Add("loading...")
	assert.NotNil(t, l.s)

	g.Start()
	assert.True(t, g.sm.Running())

	l.Update("updating...")
	l.Complete("done")

	l2 := g.Add("another...")
	l2.Error("failed")

	g.Stop()
	assert.False(t, g.sm.Running())
}
