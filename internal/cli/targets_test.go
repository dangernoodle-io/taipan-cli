package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectDevice_HostOnly(t *testing.T) {
	d := directDevice("tdongles3-3.local")
	assert.Equal(t, "tdongles3-3.local", d.Hostname)
	assert.Equal(t, "tdongles3-3.local", d.IP)
	assert.Equal(t, 80, d.Port)
}

func TestDirectDevice_HostPort(t *testing.T) {
	d := directDevice("host.local:8080")
	assert.Equal(t, "host.local", d.Hostname)
	assert.Equal(t, "host.local", d.IP)
	assert.Equal(t, 8080, d.Port)
}

func TestDirectDevice_TrailingDot(t *testing.T) {
	d := directDevice("host.local.")
	assert.Equal(t, "host.local", d.Hostname)
	assert.Equal(t, "host.local", d.IP)
	assert.Equal(t, 80, d.Port)
}

// TestResolveTargets_FastPath verifies that explicit hosts bypass mDNS discovery.
// The result must contain exactly one device with the expected fields, and since
// Browse is never called, the test completes instantly (no network I/O).
func TestResolveTargets_FastPath(t *testing.T) {
	devices, err := resolveTargets([]string{"h.local"}, false, 5)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "h.local", devices[0].IP)
	assert.Equal(t, 80, devices[0].Port)
}

// TestResolveTargets_FastPath_HostnameSorted verifies that explicit --host values
// are returned in hostname order regardless of input order.
func TestResolveTargets_FastPath_HostnameSorted(t *testing.T) {
	devices, err := resolveTargets([]string{"b.local", "a.local"}, false, 5)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, "a.local", devices[0].Hostname, "first device must be a.local")
	assert.Equal(t, "b.local", devices[1].Hostname, "second device must be b.local")
}
