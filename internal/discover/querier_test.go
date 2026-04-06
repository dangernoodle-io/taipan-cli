package discover

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/grandcat/zeroconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for parseTXT function
func TestParseTXT_Empty(t *testing.T) {
	result := parseTXT([]string{})
	assert.Equal(t, map[string]string{}, result)
}

func TestParseTXT_SingleRecord(t *testing.T) {
	result := parseTXT([]string{"board=tdongle-s3"})
	assert.Equal(t, map[string]string{"board": "tdongle-s3"}, result)
}

func TestParseTXT_MultipleRecords(t *testing.T) {
	records := []string{
		"board=tdongle-s3",
		"version=1.0.0",
		"mac=aa:bb:cc:dd:ee:ff",
	}
	result := parseTXT(records)
	expected := map[string]string{
		"board":   "tdongle-s3",
		"version": "1.0.0",
		"mac":     "aa:bb:cc:dd:ee:ff",
	}
	assert.Equal(t, expected, result)
}

func TestParseTXT_NoEquals(t *testing.T) {
	records := []string{
		"validkey=value",
		"invalidkey",
		"anotherkey=anothervalue",
	}
	result := parseTXT(records)
	expected := map[string]string{
		"validkey":    "value",
		"anotherkey":  "anothervalue",
	}
	assert.Equal(t, expected, result)
}

func TestParseTXT_ValueWithEquals(t *testing.T) {
	records := []string{"url=http://example.com?a=1"}
	result := parseTXT(records)
	assert.Equal(t, map[string]string{"url": "http://example.com?a=1"}, result)
}

// Tests for deviceFromEntry function
func TestDeviceFromEntry_BasicFields(t *testing.T) {
	// Start a test HTTP server that returns 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Parse server URL to get IP/port so the entry points at the test server
	host := srv.Listener.Addr().String()
	hostStr, portStr, err := net.SplitHostPort(host)
	require.NoError(t, err)
	testIP := net.ParseIP(hostStr)
	testPort, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	// Create a test ServiceEntry pointing at test server
	entry := &zeroconf.ServiceEntry{
		HostName: "test-device.local.",
		Port:     testPort,
		AddrIPv4: []net.IP{testIP},
		Text: []string{
			"board=tdongle-s3",
			"version=1.0.0",
			"mac=aa:bb:cc:dd:ee:ff",
		},
	}

	// Call deviceFromEntry — HTTP returns 404, so only TXT values used
	device := deviceFromEntry(entry)

	// Verify fields are populated from TXT records
	assert.Equal(t, "test-device.local.", device.Hostname)
	assert.Equal(t, testPort, device.Port)
	assert.Equal(t, "127.0.0.1", device.IP)
	assert.Equal(t, "tdongle-s3", device.Board)
	assert.Equal(t, "1.0.0", device.Version)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", device.MAC)
	assert.Equal(t, "", device.Worker) // Not in TXT records or HTTP response
}

func TestDeviceFromEntry_HTTPEnrichment(t *testing.T) {
	// Start a test HTTP server that returns JSON
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiInfoResponse{
			Board:      "bitaxe-601",
			Version:    "2.0.0",
			MAC:        "11:22:33:44:55:66",
			WorkerName: "test-worker",
		})
	}))
	defer srv.Close()

	// Parse server URL to extract port
	host := srv.Listener.Addr().String()
	hostStr, portStr, err := net.SplitHostPort(host)
	require.NoError(t, err)
	testIP := net.ParseIP(hostStr)
	testPort, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	// Create ServiceEntry pointing to test server
	entry := &zeroconf.ServiceEntry{
		HostName: "test-device.local.",
		Port:     testPort,
		AddrIPv4: []net.IP{testIP},
		Text: []string{
			"board=tdongle-s3",
			"version=1.0.0",
			"mac=aa:bb:cc:dd:ee:ff",
		},
	}

	// Save original httpClient and replace with test client
	originalClient := httpClient
	defer func() { httpClient = originalClient }()
	httpClient = srv.Client()

	// Call deviceFromEntry
	device := deviceFromEntry(entry)

	// Verify HTTP values override TXT values
	assert.Equal(t, "127.0.0.1", device.IP)
	assert.Equal(t, testPort, device.Port)
	assert.Equal(t, "bitaxe-601", device.Board)    // overridden from HTTP
	assert.Equal(t, "2.0.0", device.Version)        // overridden from HTTP
	assert.Equal(t, "11:22:33:44:55:66", device.MAC) // overridden from HTTP
	assert.Equal(t, "test-worker", device.Worker)   // populated from HTTP
}

func TestDeviceFromEntry_NoIPv4(t *testing.T) {
	// Create ServiceEntry with empty AddrIPv4
	entry := &zeroconf.ServiceEntry{
		HostName: "test-device.local.",
		Port:     8080,
		AddrIPv4: []net.IP{}, // Empty
		Text: []string{
			"board=tdongle-s3",
			"version=1.0.0",
			"mac=aa:bb:cc:dd:ee:ff",
		},
	}

	// HTTP client not needed since IP is empty; no HTTP call should be made
	device := deviceFromEntry(entry)

	// Verify TXT values are still populated and IP is empty
	assert.Equal(t, "test-device.local.", device.Hostname)
	assert.Equal(t, 8080, device.Port)
	assert.Equal(t, "", device.IP)
	assert.Equal(t, "tdongle-s3", device.Board)
	assert.Equal(t, "1.0.0", device.Version)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", device.MAC)
}
