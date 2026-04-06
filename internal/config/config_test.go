package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBoardEntryUnmarshalYAML_True(t *testing.T) {
	var be BoardEntry
	err := yaml.Unmarshal([]byte("true"), &be)
	require.NoError(t, err)
	assert.True(t, be.IsDefault)
	assert.Empty(t, be.PoolHost)
	assert.Zero(t, be.PoolPort)
}

func TestBoardEntryUnmarshalYAML_Map(t *testing.T) {
	yamlStr := `
pool_host: pool.example.com
pool_port: 3335
wallet: example-wallet
`
	var be BoardEntry
	err := yaml.Unmarshal([]byte(yamlStr), &be)
	require.NoError(t, err)
	assert.False(t, be.IsDefault)
	assert.Equal(t, "pool.example.com", be.PoolHost)
	assert.Equal(t, uint16(3335), be.PoolPort)
	assert.Equal(t, "example-wallet", be.Wallet)
}

func TestBoardEntryMarshalYAML_True(t *testing.T) {
	be := &BoardEntry{IsDefault: true}
	data, err := yaml.Marshal(be)
	require.NoError(t, err)
	assert.Equal(t, "\"true\"\n", string(data))
}

func TestBoardEntryMarshalYAML_Map(t *testing.T) {
	suffix := ""
	be := &BoardEntry{
		PoolHost:     "pool.example.com",
		PoolPort:     3335,
		Wallet:       "example-wallet",
		WorkerSuffix: &suffix,
	}
	data, err := yaml.Marshal(be)
	require.NoError(t, err)

	// Unmarshal back to verify round-trip
	var be2 BoardEntry
	err = yaml.Unmarshal(data, &be2)
	require.NoError(t, err)
	assert.Equal(t, "pool.example.com", be2.PoolHost)
	assert.Equal(t, uint16(3335), be2.PoolPort)
	assert.Equal(t, "example-wallet", be2.Wallet)
	assert.NotNil(t, be2.WorkerSuffix)
	assert.Equal(t, "", *be2.WorkerSuffix)
}

func TestLoadSaveRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	suffix := "v0"
	original := &Config{
		Profiles: map[string]*Profile{
			"home-lab": {
				WifiSSID:     "home-network",
				WifiPassword: "password123",
				PoolHost:     "pool.example.com",
				PoolPort:     3333,
				Wallet:       "home-wallet",
				WorkerPrefix: "lab",
				WorkerSuffix: &suffix,
				Boards: map[string]*BoardEntry{
					"tdongle-s3": {
						IsDefault: true,
					},
					"devkit-usb": {
						PoolPort: 3335,
					},
				},
			},
		},
	}

	// Save
	err := Save(cfgPath, original)
	require.NoError(t, err)

	// Load
	loaded, err := Load(cfgPath)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, original.Profiles["home-lab"].WifiSSID, loaded.Profiles["home-lab"].WifiSSID)
	assert.Equal(t, original.Profiles["home-lab"].PoolPort, loaded.Profiles["home-lab"].PoolPort)
	assert.Equal(t, original.Profiles["home-lab"].WorkerSuffix, loaded.Profiles["home-lab"].WorkerSuffix)

	// Verify board entries
	assert.True(t, loaded.Profiles["home-lab"].Boards["tdongle-s3"].IsDefault)
	assert.Equal(t, uint16(3335), loaded.Profiles["home-lab"].Boards["devkit-usb"].PoolPort)
}

func TestGetProfile_Found(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test-profile": {
				WifiSSID: "test-ssid",
			},
		},
	}

	profile, err := cfg.GetProfile("test-profile")
	require.NoError(t, err)
	assert.Equal(t, "test-ssid", profile.WifiSSID)
}

func TestGetProfile_NotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}

	_, err := cfg.GetProfile("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetProfile_NilProfiles(t *testing.T) {
	cfg := &Config{}

	_, err := cfg.GetProfile("any")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no profiles")
}

func TestResolveBoardInheritance(t *testing.T) {
	suffix := "v1"
	profile := &Profile{
		WifiSSID:     "home-net",
		WifiPassword: "pass123",
		PoolHost:     "pool.example.com",
		PoolPort:     3333,
		Wallet:       "main-wallet",
		WorkerPrefix: "worker",
		WorkerSuffix: &suffix,
		Boards: map[string]*BoardEntry{
			"board-a": {IsDefault: true},
			"board-b": {PoolPort: 3335},
		},
	}

	// Test board-a (all defaults)
	resolved, err := ResolveBoard(profile, "board-a")
	require.NoError(t, err)
	assert.Equal(t, "home-net", resolved.WifiSSID)
	assert.Equal(t, "pool.example.com", resolved.PoolHost)
	assert.Equal(t, uint16(3333), resolved.PoolPort)
	assert.Equal(t, "main-wallet", resolved.Wallet)
	assert.Equal(t, "worker-v1", resolved.Worker)

	// Test board-b (override pool_port)
	resolved, err = ResolveBoard(profile, "board-b")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), resolved.PoolPort)
	assert.Equal(t, "main-wallet", resolved.Wallet) // inherited
}

func TestResolveBoardWorkerName(t *testing.T) {
	suffix := "v1"
	profile := &Profile{
		WifiSSID:     "net",
		WifiPassword: "pass",
		PoolHost:     "pool.com",
		PoolPort:     3333,
		Wallet:       "wallet",
		WorkerPrefix: "pre",
		WorkerSuffix: &suffix,
		Boards: map[string]*BoardEntry{
			"board-a": {
				WorkerName: "custom-worker",
			},
		},
	}

	resolved, err := ResolveBoard(profile, "board-a")
	require.NoError(t, err)
	assert.Equal(t, "custom-worker", resolved.Worker)
}

func TestResolveBoardWorkerSuffixOverride(t *testing.T) {
	profileSuffix := "v1"
	boardSuffix := "v2"
	profile := &Profile{
		WifiSSID:     "net",
		WifiPassword: "pass",
		PoolHost:     "pool.com",
		PoolPort:     3333,
		Wallet:       "wallet",
		WorkerPrefix: "pre",
		WorkerSuffix: &profileSuffix,
		Boards: map[string]*BoardEntry{
			"board-a": {
				WorkerSuffix: &boardSuffix,
			},
		},
	}

	resolved, err := ResolveBoard(profile, "board-a")
	require.NoError(t, err)
	assert.Equal(t, "pre-v2", resolved.Worker)
}

func TestResolveBoardWorkerSuffixEmpty(t *testing.T) {
	emptySuffix := ""
	profile := &Profile{
		WorkerPrefix: "myworker",
		WorkerSuffix: &emptySuffix,
		Boards: map[string]*BoardEntry{
			"board-a": {IsDefault: true},
		},
	}

	resolved, err := ResolveBoard(profile, "board-a")
	require.NoError(t, err)
	assert.Equal(t, "myworker", resolved.Worker) // no trailing dash
}

func TestResolveBoardWorkerSuffixNil(t *testing.T) {
	profile := &Profile{
		WorkerPrefix: "pre",
		WorkerSuffix: nil, // nil means prompt later
		Boards: map[string]*BoardEntry{
			"board-a": {IsDefault: true},
		},
	}

	resolved, err := ResolveBoard(profile, "board-a")
	require.NoError(t, err)
	assert.Empty(t, resolved.Worker) // empty = will prompt
}

func TestResolveBoardNotFound(t *testing.T) {
	profile := &Profile{
		Boards: map[string]*BoardEntry{},
	}

	_, err := ResolveBoard(profile, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetProfileLevelFields(t *testing.T) {
	suffix := "test"
	profile := &Profile{
		WifiSSID:     "test-ssid",
		WifiPassword: "test-pass",
		PoolHost:     "pool.test.com",
		PoolPort:     3333,
		Wallet:       "test-wallet",
		WorkerPrefix: "prefix",
		WorkerSuffix: &suffix,
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"wifi_ssid", "test-ssid"},
		{"wifi_password", "test-pass"},
		{"pool_host", "pool.test.com"},
		{"pool_port", "3333"},
		{"wallet", "test-wallet"},
		{"worker_prefix", "prefix"},
		{"worker_suffix", "test"},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			value, err := Get(profile, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestGetBoardLevelFields(t *testing.T) {
	emptyStr := ""
	profile := &Profile{
		Boards: map[string]*BoardEntry{
			"board-a": {
				IsDefault:    true,
				PoolHost:     "board-pool.com",
				PoolPort:     3335,
				Wallet:       "board-wallet",
				WorkerName:   "board-worker",
				WorkerSuffix: &emptyStr,
			},
		},
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"boards.board-a", "true"},
		{"boards.board-a.pool_host", "board-pool.com"},
		{"boards.board-a.pool_port", "3335"},
		{"boards.board-a.wallet", "board-wallet"},
		{"boards.board-a.worker_name", "board-worker"},
		{"boards.board-a.worker_suffix", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			value, err := Get(profile, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestGetBoardNotFound(t *testing.T) {
	profile := &Profile{
		Boards: map[string]*BoardEntry{},
	}

	_, err := Get(profile, "boards.nonexistent.pool_port")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetInvalidKey(t *testing.T) {
	profile := &Profile{}

	tests := []string{
		"invalid_field",
		"wifi_ssid.nested",
		"boards",
		"",
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := Get(profile, key)
			assert.Error(t, err)
		})
	}
}

func TestSetProfileLevelFields(t *testing.T) {
	profile := &Profile{}

	tests := []struct {
		key   string
		value string
		check func(*Profile) bool
	}{
		{
			"wifi_ssid",
			"new-ssid",
			func(p *Profile) bool { return p.WifiSSID == "new-ssid" },
		},
		{
			"pool_port",
			"3335",
			func(p *Profile) bool { return p.PoolPort == 3335 },
		},
		{
			"worker_suffix",
			"v2",
			func(p *Profile) bool { return p.WorkerSuffix != nil && *p.WorkerSuffix == "v2" },
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			err := Set(profile, tc.key, tc.value)
			require.NoError(t, err)
			assert.True(t, tc.check(profile))
		})
	}
}

func TestSetBoardLevelFields(t *testing.T) {
	profile := &Profile{
		Boards: make(map[string]*BoardEntry),
	}

	err := Set(profile, "boards.dev.pool_host", "custom-pool.com")
	require.NoError(t, err)
	assert.Equal(t, "custom-pool.com", profile.Boards["dev"].PoolHost)

	err = Set(profile, "boards.dev.pool_port", "3335")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), profile.Boards["dev"].PoolPort)

	err = Set(profile, "boards.dev.worker_name", "custom-worker")
	require.NoError(t, err)
	assert.Equal(t, "custom-worker", profile.Boards["dev"].WorkerName)
}

func TestSetCreatesBoard(t *testing.T) {
	profile := &Profile{}

	err := Set(profile, "boards.new-board.wallet", "test-wallet")
	require.NoError(t, err)
	assert.NotNil(t, profile.Boards)
	assert.NotNil(t, profile.Boards["new-board"])
	assert.Equal(t, "test-wallet", profile.Boards["new-board"].Wallet)
}

func TestSetInvalidPortValue(t *testing.T) {
	profile := &Profile{}

	err := Set(profile, "pool_port", "not-a-number")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool_port")
}

func TestDefaultPath(t *testing.T) {
	// Just verify it doesn't error and returns a reasonable path
	path, err := DefaultPath()
	require.NoError(t, err)
	assert.Contains(t, path, ".config")
	assert.Contains(t, path, "taipan")
	assert.Contains(t, path, "config.yml")
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	assert.Error(t, err)
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")
	err := os.WriteFile(cfgPath, []byte("invalid: yaml: content: ["), 0o644)
	require.NoError(t, err)

	_, err = Load(cfgPath)
	assert.Error(t, err)
}

func TestSaveCreatesParentDirs(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "deep", "nested", "dirs", "config.yml")

	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {WifiSSID: "test"},
		},
	}

	err := Save(cfgPath, cfg)
	require.NoError(t, err)

	// Verify file exists and can be loaded
	loaded, err := Load(cfgPath)
	require.NoError(t, err)
	assert.NotNil(t, loaded.Profiles["test"])
}

func TestSaveFilePermissions(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	cfg := &Config{}
	err := Save(cfgPath, cfg)
	require.NoError(t, err)

	// Check file permissions
	info, err := os.Stat(cfgPath)
	require.NoError(t, err)
	mode := info.Mode()
	assert.Equal(t, os.FileMode(0o644), mode.Perm())
}
