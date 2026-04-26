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
	boardPrefix := "board-prefix"
	original := &Config{
		Wifi: &Wifi{
			SSID:     "global-ssid",
			Password: "global-pass",
		},
		Worker: &Worker{
			Suffix: &suffix,
		},
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
						PoolPort:     3335,
						WorkerPrefix: boardPrefix,
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

	// Verify global fields
	assert.NotNil(t, loaded.Wifi)
	assert.Equal(t, "global-ssid", loaded.Wifi.SSID)
	assert.Equal(t, "global-pass", loaded.Wifi.Password)
	assert.NotNil(t, loaded.Worker)
	assert.Equal(t, suffix, *loaded.Worker.Suffix)

	// Verify profile
	assert.Equal(t, original.Profiles["home-lab"].WifiSSID, loaded.Profiles["home-lab"].WifiSSID)
	assert.Equal(t, original.Profiles["home-lab"].PoolPort, loaded.Profiles["home-lab"].PoolPort)
	assert.Equal(t, original.Profiles["home-lab"].WorkerSuffix, loaded.Profiles["home-lab"].WorkerSuffix)

	// Verify board entries
	assert.True(t, loaded.Profiles["home-lab"].Boards["tdongle-s3"].IsDefault)
	assert.Equal(t, uint16(3335), loaded.Profiles["home-lab"].Boards["devkit-usb"].PoolPort)
	assert.Equal(t, boardPrefix, loaded.Profiles["home-lab"].Boards["devkit-usb"].WorkerPrefix)
}

func TestLoadSaveYAMLIndent(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	cfg := &Config{
		Wifi: &Wifi{
			SSID:     "test-ssid",
			Password: "test-pass",
		},
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.example.com",
				Wallet:   "test-wallet",
				Boards: map[string]*BoardEntry{
					"board-a": {IsDefault: true},
				},
			},
		},
	}

	// Save
	err := Save(cfgPath, cfg)
	require.NoError(t, err)

	// Read raw bytes
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	// Verify 2-space indent (no tabs)
	assert.NotContains(t, string(data), "\t", "YAML should use spaces, not tabs")
	// Verify file contains expected lines with 2-space indent
	content := string(data)
	assert.Contains(t, content, "wifi:", "should contain wifi key")
	assert.Contains(t, content, "  ssid:", "should use 2-space indent for nested keys")
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
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
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
			},
		},
	}

	// Test board-a (all defaults)
	resolved, err := ResolveBoard(cfg, "test", "board-a")
	require.NoError(t, err)
	assert.Equal(t, "home-net", resolved.WifiSSID)
	assert.Equal(t, "pool.example.com", resolved.PoolHost)
	assert.Equal(t, uint16(3333), resolved.PoolPort)
	assert.Equal(t, "main-wallet", resolved.Wallet)
	assert.Equal(t, "worker-v1", resolved.Worker)

	// Test board-b (override pool_port)
	resolved, err = ResolveBoard(cfg, "test", "board-b")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), resolved.PoolPort)
	assert.Equal(t, "main-wallet", resolved.Wallet) // inherited
}

func TestResolveBoardWorkerName(t *testing.T) {
	suffix := "v1"
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
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
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board-a")
	require.NoError(t, err)
	assert.Equal(t, "custom-worker", resolved.Worker)
}

func TestResolveBoardWorkerSuffixOverride(t *testing.T) {
	profileSuffix := "v1"
	boardSuffix := "v2"
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
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
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board-a")
	require.NoError(t, err)
	assert.Equal(t, "pre-v2", resolved.Worker)
}

func TestResolveBoardWorkerSuffixEmpty(t *testing.T) {
	emptySuffix := ""
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				WorkerPrefix: "myworker",
				WorkerSuffix: &emptySuffix,
				Boards: map[string]*BoardEntry{
					"board-a": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board-a")
	require.NoError(t, err)
	assert.Equal(t, "myworker", resolved.Worker) // no trailing dash
}

func TestResolveBoardWorkerSuffixNil(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				WorkerPrefix: "pre",
				WorkerSuffix: nil, // nil means prompt later
				Boards: map[string]*BoardEntry{
					"board-a": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board-a")
	require.NoError(t, err)
	assert.Empty(t, resolved.Worker) // empty = will prompt
}

func TestResolveBoardNotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{},
			},
		},
	}

	_, err := ResolveBoard(cfg, "test", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetProfileLevelFields(t *testing.T) {
	suffix := "test"
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				WifiSSID:     "test-ssid",
				WifiPassword: "test-pass",
				PoolHost:     "pool.test.com",
				PoolPort:     3333,
				Wallet:       "test-wallet",
				WorkerPrefix: "prefix",
				WorkerSuffix: &suffix,
			},
		},
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
			value, err := Get(cfg, "test", tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestGetBoardLevelFields(t *testing.T) {
	emptyStr := ""
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{
					"board-a": {
						IsDefault:    true,
						PoolHost:     "board-pool.com",
						PoolPort:     3335,
						Wallet:       "board-wallet",
						WorkerName:   "board-worker",
						WorkerPrefix: "board-prefix",
						WorkerSuffix: &emptyStr,
					},
				},
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
		{"boards.board-a.worker_prefix", "board-prefix"},
		{"boards.board-a.worker_suffix", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			value, err := Get(cfg, "test", tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestGetBoardNotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{},
			},
		},
	}

	_, err := Get(cfg, "test", "boards.nonexistent.pool_port")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetInvalidKey(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	tests := []string{
		"invalid_field",
		"wifi_ssid.nested",
		"boards",
		"",
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := Get(cfg, "test", key)
			assert.Error(t, err)
		})
	}
}

func TestSetProfileLevelFields(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	tests := []struct {
		key   string
		value string
		check func(*Config) bool
	}{
		{
			"wifi_ssid",
			"new-ssid",
			func(c *Config) bool { return c.Profiles["test"].WifiSSID == "new-ssid" },
		},
		{
			"pool_port",
			"3335",
			func(c *Config) bool { return c.Profiles["test"].PoolPort == 3335 },
		},
		{
			"worker_suffix",
			"v2",
			func(c *Config) bool {
				return c.Profiles["test"].WorkerSuffix != nil && *c.Profiles["test"].WorkerSuffix == "v2"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			err := Set(cfg, "test", tc.key, tc.value)
			require.NoError(t, err)
			assert.True(t, tc.check(cfg))
		})
	}
}

func TestSetBoardLevelFields(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: make(map[string]*BoardEntry),
			},
		},
	}

	err := Set(cfg, "test", "boards.dev.pool_host", "custom-pool.com")
	require.NoError(t, err)
	assert.Equal(t, "custom-pool.com", cfg.Profiles["test"].Boards["dev"].PoolHost)

	err = Set(cfg, "test", "boards.dev.pool_port", "3335")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), cfg.Profiles["test"].Boards["dev"].PoolPort)

	err = Set(cfg, "test", "boards.dev.worker_name", "custom-worker")
	require.NoError(t, err)
	assert.Equal(t, "custom-worker", cfg.Profiles["test"].Boards["dev"].WorkerName)

	err = Set(cfg, "test", "boards.dev.worker_prefix", "board-prefix")
	require.NoError(t, err)
	assert.Equal(t, "board-prefix", cfg.Profiles["test"].Boards["dev"].WorkerPrefix)
}

func TestSetCreatesBoard(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "boards.new-board.wallet", "test-wallet")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Profiles["test"].Boards)
	assert.NotNil(t, cfg.Profiles["test"].Boards["new-board"])
	assert.Equal(t, "test-wallet", cfg.Profiles["test"].Boards["new-board"].Wallet)
}

func TestSetInvalidPortValue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "pool_port", "not-a-number")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool_port")
}

func TestGetConfigLevelWifi(t *testing.T) {
	cfg := &Config{
		Wifi: &Wifi{
			SSID:     "global-ssid",
			Password: "global-pass",
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	ssid, err := Get(cfg, "test", "wifi.ssid")
	require.NoError(t, err)
	assert.Equal(t, "global-ssid", ssid)

	pass, err := Get(cfg, "test", "wifi.password")
	require.NoError(t, err)
	assert.Equal(t, "global-pass", pass)
}

func TestGetConfigLevelWorker(t *testing.T) {
	suffix := "global-v1"
	cfg := &Config{
		Worker: &Worker{
			Suffix: &suffix,
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	val, err := Get(cfg, "test", "worker.suffix")
	require.NoError(t, err)
	assert.Equal(t, "global-v1", val)
}

func TestSetConfigLevelWifi(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "wifi.ssid", "new-ssid")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Wifi)
	assert.Equal(t, "new-ssid", cfg.Wifi.SSID)

	err = Set(cfg, "test", "wifi.password", "new-pass")
	require.NoError(t, err)
	assert.Equal(t, "new-pass", cfg.Wifi.Password)
}

func TestSetConfigLevelWorker(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "worker.suffix", "new-suffix")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Worker)
	assert.Equal(t, "new-suffix", *cfg.Worker.Suffix)
}

func TestResolveBoardWifiPrecedence(t *testing.T) {
	globalSSID := "global-ssid"
	globalPass := "global-pass"
	cfg := &Config{
		Wifi: &Wifi{
			SSID:     globalSSID,
			Password: globalPass,
		},
		Profiles: map[string]*Profile{
			"profile-with-wifi": {
				WifiSSID:     "profile-ssid",
				WifiPassword: "profile-pass",
				PoolHost:     "pool.com",
				Wallet:       "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
			"profile-no-wifi": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	// Profile wifi overrides global
	resolved, err := ResolveBoard(cfg, "profile-with-wifi", "board")
	require.NoError(t, err)
	assert.Equal(t, "profile-ssid", resolved.WifiSSID)
	assert.Equal(t, "profile-pass", resolved.WifiPassword)

	// Global wifi used when profile empty
	resolved, err = ResolveBoard(cfg, "profile-no-wifi", "board")
	require.NoError(t, err)
	assert.Equal(t, globalSSID, resolved.WifiSSID)
	assert.Equal(t, globalPass, resolved.WifiPassword)
}

func TestResolveBoardWorkerPrefixPrecedence(t *testing.T) {
	boardPrefix := "board-prefix"
	profilePrefix := "profile-prefix"
	suffix := "v1"
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				WorkerPrefix: profilePrefix,
				WorkerSuffix: &suffix,
				PoolHost:     "pool.com",
				Wallet:       "wallet",
				Boards: map[string]*BoardEntry{
					"board-with-prefix": {
						WorkerPrefix: boardPrefix,
					},
					"board-no-prefix": {IsDefault: true},
				},
			},
		},
	}

	// Board prefix overrides profile
	resolved, err := ResolveBoard(cfg, "test", "board-with-prefix")
	require.NoError(t, err)
	assert.Equal(t, "board-prefix-v1", resolved.Worker)

	// Profile prefix used when board empty
	resolved, err = ResolveBoard(cfg, "test", "board-no-prefix")
	require.NoError(t, err)
	assert.Equal(t, "profile-prefix-v1", resolved.Worker)
}

func TestResolveBoardGlobalWorkerSuffix(t *testing.T) {
	globalSuffix := "global-v1"
	cfg := &Config{
		Worker: &Worker{
			Suffix: &globalSuffix,
		},
		Profiles: map[string]*Profile{
			"test": {
				WorkerPrefix: "prefix",
				PoolHost:     "pool.com",
				Wallet:       "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board")
	require.NoError(t, err)
	// Profile has no suffix, should use global
	assert.Equal(t, "prefix-global-v1", resolved.Worker)
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

// New tests for Round-2 additions: Pool, DisplayEn, 4-tier resolution

func TestLoadSavePoolRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	original := &Config{
		Pool: &Pool{
			Host:     "pool.example.com",
			Port:     3333,
			Password: "secret",
		},
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
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

	// Verify pool
	assert.NotNil(t, loaded.Pool)
	assert.Equal(t, "pool.example.com", loaded.Pool.Host)
	assert.Equal(t, uint16(3333), loaded.Pool.Port)
	assert.Equal(t, "secret", loaded.Pool.Password)
}

func TestLoadSaveGlobalBoardsRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	prefix := "global-prefix"
	original := &Config{
		Boards: map[string]*BoardEntry{
			"bitaxe-403": {
				PoolHost:     "bch.hmpool.io",
				PoolPort:     3335,
				PoolPassword: "x",
				WorkerPrefix: prefix,
			},
		},
		Profiles: map[string]*Profile{
			"bch": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"bitaxe-403": {IsDefault: true},
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

	// Verify global boards
	assert.NotNil(t, loaded.Boards)
	assert.NotNil(t, loaded.Boards["bitaxe-403"])
	assert.Equal(t, "bch.hmpool.io", loaded.Boards["bitaxe-403"].PoolHost)
	assert.Equal(t, uint16(3335), loaded.Boards["bitaxe-403"].PoolPort)
	assert.Equal(t, prefix, loaded.Boards["bitaxe-403"].WorkerPrefix)
}

func TestLoadSaveWorkerPrefixRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	globalPrefix := "global-prefix"
	original := &Config{
		Worker: &Worker{
			Prefix: &globalPrefix,
			Suffix: nil,
		},
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
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

	// Verify worker.prefix
	assert.NotNil(t, loaded.Worker)
	assert.NotNil(t, loaded.Worker.Prefix)
	assert.Equal(t, globalPrefix, *loaded.Worker.Prefix)
}

func TestLoadSaveDisplayEnRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	trueVal := true
	falseVal := false
	original := &Config{
		Boards: map[string]*BoardEntry{
			"board-a": {
				DisplayEn: &trueVal,
			},
			"board-b": {
				DisplayEn: &falseVal,
			},
		},
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board-a": {IsDefault: true},
					"board-b": {IsDefault: true},
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

	// Verify display_en
	assert.NotNil(t, loaded.Boards["board-a"].DisplayEn)
	assert.True(t, *loaded.Boards["board-a"].DisplayEn)
	assert.NotNil(t, loaded.Boards["board-b"].DisplayEn)
	assert.False(t, *loaded.Boards["board-b"].DisplayEn)
}

func TestResolveBoardPoolFourTierChain(t *testing.T) {
	// Test 4-tier chain: profile.board > profile > cfg.boards[board] > cfg.pool
	cfg := &Config{
		Pool: &Pool{
			Host:     "global-pool.com",
			Port:     3333,
			Password: "global-pass",
		},
		Boards: map[string]*BoardEntry{
			"bitaxe": {
				PoolHost:     "global-boards-pool.com",
				PoolPort:     3334,
				PoolPassword: "board-pass",
			},
		},
		Profiles: map[string]*Profile{
			"bch": {
				PoolHost:     "profile-pool.com",
				PoolPort:     3335,
				PoolPassword: "profile-pass",
				Wallet:       "bch-wallet",
				Boards: map[string]*BoardEntry{
					"bitaxe": {
						PoolHost:     "board-override-pool.com",
						PoolPort:     3336,
						PoolPassword: "board-override-pass",
					},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "bch", "bitaxe")
	require.NoError(t, err)

	// Verify 4-tier chain: profile.board wins
	assert.Equal(t, "board-override-pool.com", resolved.PoolHost)
	assert.Equal(t, uint16(3336), resolved.PoolPort)
	assert.Equal(t, "board-override-pass", resolved.PoolPassword)
}

func TestResolveBoardPoolFallthrough(t *testing.T) {
	// Test fallthrough: profile.board empty, so use profile
	cfg := &Config{
		Pool: &Pool{
			Host:     "global-pool.com",
			Port:     3333,
			Password: "global-pass",
		},
		Boards: map[string]*BoardEntry{
			"bitaxe": {
				PoolHost:     "global-boards-pool.com",
				PoolPort:     3334,
				PoolPassword: "board-pass",
			},
		},
		Profiles: map[string]*Profile{
			"bch": {
				PoolHost:     "profile-pool.com",
				PoolPort:     3335,
				PoolPassword: "profile-pass",
				Wallet:       "bch-wallet",
				Boards: map[string]*BoardEntry{
					"bitaxe": {IsDefault: true}, // no overrides
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "bch", "bitaxe")
	require.NoError(t, err)

	// Verify fallthrough to profile
	assert.Equal(t, "profile-pool.com", resolved.PoolHost)
	assert.Equal(t, uint16(3335), resolved.PoolPort)
	assert.Equal(t, "profile-pass", resolved.PoolPassword)
}

func TestResolveBoardPoolDefaultPassword(t *testing.T) {
	// Test default password "x" when all empty
	cfg := &Config{
		Pool: &Pool{
			Host: "pool.com",
			Port: 3333,
		},
		Profiles: map[string]*Profile{
			"bch": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"bitaxe": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "bch", "bitaxe")
	require.NoError(t, err)

	// Verify default password
	assert.Equal(t, "x", resolved.PoolPassword)
}

func TestResolveBoardDisplayEnDefault(t *testing.T) {
	// Test default display_en = true
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board")
	require.NoError(t, err)

	// Verify default true
	assert.True(t, resolved.DisplayEn)
}

func TestResolveBoardDisplayEnFalse(t *testing.T) {
	// Test display_en=false override
	falseVal := false
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"board": {
				DisplayEn: &falseVal,
			},
		},
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board")
	require.NoError(t, err)

	// Verify false from global board
	assert.False(t, resolved.DisplayEn)
}

func TestResolveBoardDisplayEnBoardOverride(t *testing.T) {
	// Test board-level display_en overrides global
	boardTrue := true
	globalFalse := false
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"board": {
				DisplayEn: &globalFalse,
			},
		},
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {
						DisplayEn: &boardTrue, // board level wins
					},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board")
	require.NoError(t, err)

	// Verify board override
	assert.True(t, resolved.DisplayEn)
}

func TestResolveBoardStrictEnrollment(t *testing.T) {
	// Test strict enrollment: board must exist in profile.Boards
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"bitaxe": {
				PoolHost: "pool.com",
			},
		},
		Profiles: map[string]*Profile{
			"bch": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards:   map[string]*BoardEntry{
					// bitaxe not enrolled in this profile
				},
			},
		},
	}

	// Should fail even though bitaxe exists in cfg.Boards
	_, err := ResolveBoard(cfg, "bch", "bitaxe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetGlobalPool(t *testing.T) {
	cfg := &Config{
		Pool: &Pool{
			Host:     "pool.com",
			Port:     3333,
			Password: "pass",
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	host, err := Get(cfg, "test", "pool.host")
	require.NoError(t, err)
	assert.Equal(t, "pool.com", host)

	port, err := Get(cfg, "test", "pool.port")
	require.NoError(t, err)
	assert.Equal(t, "3333", port)

	pass, err := Get(cfg, "test", "pool.password")
	require.NoError(t, err)
	assert.Equal(t, "pass", pass)
}

func TestGetGlobalWorkerPrefix(t *testing.T) {
	prefix := "global-prefix"
	cfg := &Config{
		Worker: &Worker{
			Prefix: &prefix,
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	val, err := Get(cfg, "test", "worker.prefix")
	require.NoError(t, err)
	assert.Equal(t, "global-prefix", val)
}

func TestSetGlobalPool(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "pool.host", "new-pool.com")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Pool)
	assert.Equal(t, "new-pool.com", cfg.Pool.Host)

	err = Set(cfg, "test", "pool.port", "3335")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), cfg.Pool.Port)

	err = Set(cfg, "test", "pool.password", "secret")
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Pool.Password)
}

func TestSetGlobalWorkerPrefix(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "worker.prefix", "new-prefix")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Worker)
	assert.NotNil(t, cfg.Worker.Prefix)
	assert.Equal(t, "new-prefix", *cfg.Worker.Prefix)
}

func TestGetGlobalBoardWithEmptyProfile(t *testing.T) {
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"bitaxe": {
				PoolHost:     "pool.com",
				PoolPort:     3335,
				WorkerPrefix: "prefix",
			},
		},
		Profiles: map[string]*Profile{},
	}

	// Get with empty profileName should route to global boards
	host, err := Get(cfg, "", "boards.bitaxe.pool_host")
	require.NoError(t, err)
	assert.Equal(t, "pool.com", host)

	port, err := Get(cfg, "", "boards.bitaxe.pool_port")
	require.NoError(t, err)
	assert.Equal(t, "3335", port)

	prefix, err := Get(cfg, "", "boards.bitaxe.worker_prefix")
	require.NoError(t, err)
	assert.Equal(t, "prefix", prefix)
}

func TestGetBoardDisplayEn(t *testing.T) {
	trueVal := true
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{
					"board": {
						DisplayEn: &trueVal,
					},
				},
			},
		},
	}

	val, err := Get(cfg, "test", "boards.board.display_en")
	require.NoError(t, err)
	assert.Equal(t, "true", val)
}

func TestSetBoardDisplayEn(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: make(map[string]*BoardEntry),
			},
		},
	}

	err := Set(cfg, "test", "boards.board.display_en", "false")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Profiles["test"].Boards["board"].DisplayEn)
	assert.False(t, *cfg.Profiles["test"].Boards["board"].DisplayEn)
}

func TestSetBoardPoolPassword(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: make(map[string]*BoardEntry),
			},
		},
	}

	err := Set(cfg, "test", "boards.board.pool_password", "secret")
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Profiles["test"].Boards["board"].PoolPassword)
}

func TestSetGlobalBoardWithEmptyProfile(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}

	// Set with empty profileName should route to global boards
	err := Set(cfg, "", "boards.bitaxe.pool_host", "pool.com")
	require.NoError(t, err)
	assert.Equal(t, "pool.com", cfg.Boards["bitaxe"].PoolHost)

	err = Set(cfg, "", "boards.bitaxe.pool_password", "secret")
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Boards["bitaxe"].PoolPassword)
}

func TestGetProfilePoolPassword(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				PoolPassword: "profile-pass",
			},
		},
	}

	val, err := Get(cfg, "test", "pool_password")
	require.NoError(t, err)
	assert.Equal(t, "profile-pass", val)
}

func TestSetProfilePoolPassword(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "pool_password", "new-pass")
	require.NoError(t, err)
	assert.Equal(t, "new-pass", cfg.Profiles["test"].PoolPassword)
}

// Additional error branch tests for Set()

func TestSetNilConfig(t *testing.T) {
	err := Set(nil, "test", "pool_host", "pool.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestSetEmptyKey(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetNoProfileRequired(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "pool_host", "pool.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "profile required")
}

func TestSetProfileNotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "nonexistent", "pool_host", "pool.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSetProfileNil(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": nil,
		},
	}
	err := Set(cfg, "test", "pool_host", "pool.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestSetInvalidWifiKey(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "wifi.ssid.nested", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetInvalidWifiField(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "wifi.invalid", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid wifi field")
}

func TestSetInvalidWorkerField(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "worker.invalid", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid worker field")
}

func TestSetInvalidPoolField(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "pool.invalid", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool field")
}

func TestSetInvalidPoolPortValue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "pool.port", "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool port")
}

func TestSetInvalidBoardField(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {Boards: make(map[string]*BoardEntry)},
		},
	}
	err := Set(cfg, "test", "boards.board.invalid_field", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid board field")
}

func TestSetInvalidBoardDisplayEnValue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {Boards: make(map[string]*BoardEntry)},
		},
	}
	err := Set(cfg, "test", "boards.board.display_en", "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'true' or 'false'")
}

func TestSetGlobalBoardInvalidField(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board.invalid_field", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid board field")
}

func TestSetGlobalBoardInvalidDisplayEnValue(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board.display_en", "maybe")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'true' or 'false'")
}

func TestSetInvalidBoardPortValue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {Boards: make(map[string]*BoardEntry)},
		},
	}
	err := Set(cfg, "test", "boards.board.pool_port", "bad-port")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool_port")
}

func TestSetInvalidKeyPrefix(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	tests := []string{
		"invalid_prefix",
	}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			err := Set(cfg, "test", key, "value")
			assert.Error(t, err)
		})
	}
}

// Additional error branch tests for Get()

func TestGetNilConfig(t *testing.T) {
	_, err := Get(nil, "test", "pool_host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestGetEmptyKey(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestGetNoProfileRequired(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}
	_, err := Get(cfg, "", "pool_host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "profile required")
}

func TestGetProfileNotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}
	_, err := Get(cfg, "nonexistent", "pool_host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetProfileNil(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": nil,
		},
	}
	_, err := Get(cfg, "test", "pool_host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestGetInvalidWifiKey(t *testing.T) {
	cfg := &Config{
		Wifi: &Wifi{SSID: "test"},
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "wifi.invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid wifi field")
}

func TestGetInvalidWorkerField(t *testing.T) {
	cfg := &Config{
		Worker:   &Worker{},
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "worker.invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid worker field")
}

func TestGetInvalidPoolField(t *testing.T) {
	cfg := &Config{
		Pool:     &Pool{},
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "pool.invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool field")
}

func TestGetInvalidBoardField(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}
	_, err := Get(cfg, "test", "boards.board.invalid_field")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid board field")
}

func TestGetGlobalBoardNotFound(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	_, err := Get(cfg, "", "boards.nonexistent.pool_host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetGlobalBoardInvalidField(t *testing.T) {
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"board": {IsDefault: true},
		},
		Profiles: map[string]*Profile{},
	}
	_, err := Get(cfg, "", "boards.board.invalid_field")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid board field")
}

func TestGetInvalidKeyPrefix(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	tests := []string{
		"invalid_prefix",
	}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := Get(cfg, "test", key)
			assert.Error(t, err)
		})
	}
}

func TestGetWifiWithNilStruct(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	ssid, err := Get(cfg, "test", "wifi.ssid")
	require.NoError(t, err)
	assert.Empty(t, ssid)
}

func TestGetWorkerWithNilStruct(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	val, err := Get(cfg, "test", "worker.suffix")
	require.NoError(t, err)
	assert.Empty(t, val)
}

func TestGetPoolWithNilStruct(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	host, err := Get(cfg, "test", "pool.host")
	require.NoError(t, err)
	assert.Empty(t, host)
}

func TestGetGlobalBoardMinimalKey(t *testing.T) {
	cfg := &Config{
		Boards: map[string]*BoardEntry{
			"board": {IsDefault: true},
		},
		Profiles: map[string]*Profile{},
	}
	val, err := Get(cfg, "", "boards.board")
	require.NoError(t, err)
	assert.Equal(t, "true", val)

	cfg2 := &Config{
		Boards: map[string]*BoardEntry{
			"board": {PoolHost: "pool.com"},
		},
		Profiles: map[string]*Profile{},
	}
	val2, err := Get(cfg2, "", "boards.board")
	require.NoError(t, err)
	assert.Equal(t, "map", val2)
}

func TestGetProfileBoardMinimalKey(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}
	val, err := Get(cfg, "test", "boards.board")
	require.NoError(t, err)
	assert.Equal(t, "true", val)

	cfg2 := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{
					"board": {PoolHost: "pool.com"},
				},
			},
		},
	}
	val2, err := Get(cfg2, "test", "boards.board")
	require.NoError(t, err)
	assert.Equal(t, "map", val2)
}

// ResolveBoard error tests

func TestResolveBoardNilConfig(t *testing.T) {
	_, err := ResolveBoard(nil, "test", "board")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestResolveBoardProfileNotFound(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{},
	}
	_, err := ResolveBoard(cfg, "nonexistent", "board")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveBoardProfileNil(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": nil,
		},
	}
	_, err := ResolveBoard(cfg, "test", "board")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestResolveBoardBoardNotEnrolled(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {
				Boards: map[string]*BoardEntry{},
			},
		},
	}
	_, err := ResolveBoard(cfg, "test", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Edge cases for display_en parsing

func TestSetBoardDisplayEnTrue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {Boards: make(map[string]*BoardEntry)},
		},
	}
	err := Set(cfg, "test", "boards.board.display_en", "true")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Profiles["test"].Boards["board"].DisplayEn)
	assert.True(t, *cfg.Profiles["test"].Boards["board"].DisplayEn)
}

func TestSetGlobalBoardDisplayEnFalse(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board.display_en", "false")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Boards["board"].DisplayEn)
	assert.False(t, *cfg.Boards["board"].DisplayEn)
}

// Edge cases for port parsing

func TestSetPoolPortValid(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "pool.port", "3335")
	require.NoError(t, err)
	assert.Equal(t, uint16(3335), cfg.Pool.Port)
}

func TestSetGlobalBoardPoolPortValid(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board.pool_port", "9999")
	require.NoError(t, err)
	assert.Equal(t, uint16(9999), cfg.Boards["board"].PoolPort)
}

func TestSetGlobalBoardPoolPortInvalid(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board.pool_port", "bad-port")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool_port")
}

// Test invalid key paths that are too short

func TestGetWifiKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "wifi")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestGetWorkerKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "worker")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestGetPoolKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "pool")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestGetBoardsKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	_, err := Get(cfg, "test", "boards")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

// Test invalid key paths that are too short for Set

func TestSetWifiKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "wifi", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetWorkerKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "worker", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetPoolKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "pool", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetBoardsKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{"test": {}},
	}
	err := Set(cfg, "test", "boards", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetGlobalBoardsKeyTooShort(t *testing.T) {
	cfg := &Config{
		Boards:   make(map[string]*BoardEntry),
		Profiles: map[string]*Profile{},
	}
	err := Set(cfg, "", "boards.board", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

// Tests for Device struct round-trip, resolution, and Get/Set

func TestLoadSaveDeviceRoundTrip(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	trueVal := true
	original := &Config{
		Device: &Device{
			DisplayEn: &trueVal,
		},
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
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

	// Verify device
	assert.NotNil(t, loaded.Device)
	assert.NotNil(t, loaded.Device.DisplayEn)
	assert.True(t, *loaded.Device.DisplayEn)
}

func TestLoadSaveDeviceNilOmitted(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.yml")

	original := &Config{
		Device: nil,
		Profiles: map[string]*Profile{
			"default": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	// Save
	err := Save(cfgPath, original)
	require.NoError(t, err)

	// Read raw bytes to verify device key not present
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content := string(data)
	assert.NotContains(t, content, "device:", "device key should be omitted when nil")

	// Load and verify
	loaded, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Nil(t, loaded.Device)
}

func TestResolveBoardDeviceDisplayEnChain(t *testing.T) {
	// Test 4-tier chain: profile.board > cfg.boards[board] > cfg.device > default true
	deviceTrue := true
	boardFalse := false
	profileTrue := true

	cfg := &Config{
		Device: &Device{
			DisplayEn: &deviceTrue,
		},
		Boards: map[string]*BoardEntry{
			"board-1": {
				DisplayEn: &boardFalse,
			},
		},
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board-1": {IsDefault: true},
					"board-2": {
						DisplayEn: &profileTrue,
					},
					"board-3": {IsDefault: true},
				},
			},
		},
	}

	// board-1: profile (nil) -> global boards (false) wins
	resolved, err := ResolveBoard(cfg, "test", "board-1")
	require.NoError(t, err)
	assert.False(t, resolved.DisplayEn)

	// board-2: profile (true) wins
	resolved, err = ResolveBoard(cfg, "test", "board-2")
	require.NoError(t, err)
	assert.True(t, resolved.DisplayEn)

	// board-3: profile (nil) -> global boards (nil) -> device (true) wins
	resolved, err = ResolveBoard(cfg, "test", "board-3")
	require.NoError(t, err)
	assert.True(t, resolved.DisplayEn)
}

func TestResolveBoardDeviceDisplayEnDefault(t *testing.T) {
	// Test default true when cfg.Device is nil and global/profile boards don't set it
	cfg := &Config{
		Device: nil,
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board": {IsDefault: true},
				},
			},
		},
	}

	resolved, err := ResolveBoard(cfg, "test", "board")
	require.NoError(t, err)
	assert.True(t, resolved.DisplayEn)
}

func TestGetDeviceDisplayEn(t *testing.T) {
	trueVal := true
	cfg := &Config{
		Device: &Device{
			DisplayEn: &trueVal,
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	val, err := Get(cfg, "test", "device.display_en")
	require.NoError(t, err)
	assert.Equal(t, "true", val)
}

func TestGetDeviceDisplayEnNil(t *testing.T) {
	cfg := &Config{
		Device: &Device{
			DisplayEn: nil,
		},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	val, err := Get(cfg, "test", "device.display_en")
	require.NoError(t, err)
	assert.Empty(t, val)
}

func TestGetDeviceNil(t *testing.T) {
	cfg := &Config{
		Device: nil,
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	val, err := Get(cfg, "test", "device.display_en")
	require.NoError(t, err)
	assert.Empty(t, val)
}

func TestSetDeviceDisplayEnTrue(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device.display_en", "true")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Device)
	assert.NotNil(t, cfg.Device.DisplayEn)
	assert.True(t, *cfg.Device.DisplayEn)
}

func TestSetDeviceDisplayEnFalse(t *testing.T) {
	cfg := &Config{
		Device: &Device{},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device.display_en", "false")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Device.DisplayEn)
	assert.False(t, *cfg.Device.DisplayEn)
}

func TestSetDeviceDisplayEnInvalid(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device.display_en", "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'true' or 'false'")
}

func TestSetDeviceCreatesStruct(t *testing.T) {
	cfg := &Config{
		Device: nil,
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device.display_en", "false")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Device)
	assert.NotNil(t, cfg.Device.DisplayEn)
	assert.False(t, *cfg.Device.DisplayEn)
}

func TestGetDeviceInvalidField(t *testing.T) {
	cfg := &Config{
		Device: &Device{},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	_, err := Get(cfg, "test", "device.invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid device field")
}

func TestSetDeviceInvalidField(t *testing.T) {
	cfg := &Config{
		Device: &Device{},
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device.invalid", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid device field")
}

func TestGetDeviceKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	_, err := Get(cfg, "test", "device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestSetDeviceKeyTooShort(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			"test": {},
		},
	}

	err := Set(cfg, "test", "device", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key")
}

func TestResolveBoardDeviceDisplayEnOverridePrecedence(t *testing.T) {
	// Verify precedence: profile.board > global board > device > default
	deviceFalse := false
	boardTrue := true
	profileFalse := false

	cfg := &Config{
		Device: &Device{
			DisplayEn: &deviceFalse,
		},
		Boards: map[string]*BoardEntry{
			"board-1": {
				DisplayEn: &boardTrue,
			},
			"board-2": {
				DisplayEn: nil,
			},
		},
		Profiles: map[string]*Profile{
			"test": {
				PoolHost: "pool.com",
				Wallet:   "wallet",
				Boards: map[string]*BoardEntry{
					"board-1": {
						DisplayEn: &profileFalse,
					},
					"board-2": {IsDefault: true},
				},
			},
		},
	}

	// board-1: profile.board (false) wins over everything
	resolved, err := ResolveBoard(cfg, "test", "board-1")
	require.NoError(t, err)
	assert.False(t, resolved.DisplayEn)

	// board-2: profile.board (nil) -> global board (nil) -> device (false) wins
	resolved, err = ResolveBoard(cfg, "test", "board-2")
	require.NoError(t, err)
	assert.False(t, resolved.DisplayEn)
}

func TestResolveBoard_WorkerNameMatrix(t *testing.T) {
	// Test the full 4-tier prefix chain + 3-tier suffix chain + literal worker_name override.
	// Prefix tiers: profile.board.worker_prefix > profile.worker_prefix > cfg.boards[board].worker_prefix > cfg.worker.prefix > (none)
	// Suffix tiers: profile.board.worker_suffix > profile.worker_suffix > cfg.worker.suffix > (none)
	// WorkerName override: if set, uses verbatim and ignores prefix/suffix

	tests := []struct {
		name              string
		prefixTier        string // where prefix is set: "board", "profile", "global_board", "global_worker", "none"
		suffixTier        string // where suffix is set: "board", "profile", "global_worker", "nil", "empty"
		hasWorkerName     bool
		expectedPrefix    string
		expectedWorker    string
	}{
		// Prefix at profile.board.worker_prefix
		{"prefix_board_suffix_board", "board", "board", false, "pbp", "pbp-bbs"},
		{"prefix_board_suffix_profile", "board", "profile", false, "pbp", "pbp-pws"},
		{"prefix_board_suffix_global", "board", "global_worker", false, "pbp", "pbp-alpha"},
		{"prefix_board_suffix_nil", "board", "nil", false, "pbp", ""},
		{"prefix_board_suffix_empty", "board", "empty", false, "pbp", "pbp"},
		{"prefix_board_worker_name", "board", "profile", true, "pbp", "custom"},

		// Prefix at profile.worker_prefix
		{"prefix_profile_suffix_board", "profile", "board", false, "pws", "pws-bbs"},
		{"prefix_profile_suffix_profile", "profile", "profile", false, "pws", "pws-pws"},
		{"prefix_profile_suffix_global", "profile", "global_worker", false, "pws", "pws-alpha"},
		{"prefix_profile_suffix_nil", "profile", "nil", false, "pws", ""},
		{"prefix_profile_suffix_empty", "profile", "empty", false, "pws", "pws"},

		// Prefix at cfg.boards[board].worker_prefix
		{"prefix_global_board_suffix_board", "global_board", "board", false, "gbs", "gbs-bbs"},
		{"prefix_global_board_suffix_profile", "global_board", "profile", false, "gbs", "gbs-pws"},
		{"prefix_global_board_suffix_global", "global_board", "global_worker", false, "gbs", "gbs-alpha"},
		{"prefix_global_board_suffix_nil", "global_board", "nil", false, "gbs", ""},
		{"prefix_global_board_suffix_empty", "global_board", "empty", false, "gbs", "gbs"},

		// Prefix at cfg.worker.prefix
		{"prefix_global_worker_suffix_board", "global_worker", "board", false, "gws", "gws-bbs"},
		{"prefix_global_worker_suffix_profile", "global_worker", "profile", false, "gws", "gws-pws"},
		{"prefix_global_worker_suffix_global", "global_worker", "global_worker", false, "gws", "gws-alpha"},
		{"prefix_global_worker_suffix_nil", "global_worker", "nil", false, "gws", ""},
		{"prefix_global_worker_suffix_empty", "global_worker", "empty", false, "gws", "gws"},

		// No prefix (empty) -- suffix only becomes just the suffix value without dash
		{"no_prefix_suffix_board", "none", "board", false, "", "-bbs"},
		{"no_prefix_suffix_profile", "none", "profile", false, "", "-pws"},
		{"no_prefix_suffix_global", "none", "global_worker", false, "", "-alpha"},
		{"no_prefix_suffix_nil", "none", "nil", false, "", ""},
		{"no_prefix_suffix_empty", "none", "empty", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Profiles: map[string]*Profile{
					"test": {
						PoolHost: "pool.com",
						Wallet:   "wallet",
						Boards:   map[string]*BoardEntry{},
					},
				},
			}

			board := cfg.Profiles["test"]
			boardEntry := &BoardEntry{}

			// Set prefix according to tier
			switch tt.prefixTier {
			case "board":
				boardEntry.WorkerPrefix = "pbp"
			case "profile":
				board.WorkerPrefix = "pws"
			case "global_board":
				if cfg.Boards == nil {
					cfg.Boards = make(map[string]*BoardEntry)
				}
				cfg.Boards["board-a"] = &BoardEntry{WorkerPrefix: "gbs"}
			case "global_worker":
				gwsVal := "gws"
				cfg.Worker = &Worker{Prefix: &gwsVal}
			case "none":
				// Don't set anything
			}

			// Set suffix according to tier
			switch tt.suffixTier {
			case "board":
				bbs := "bbs"
				boardEntry.WorkerSuffix = &bbs
			case "profile":
				pws := "pws"
				board.WorkerSuffix = &pws
			case "global_worker":
				alpha := "alpha"
				if cfg.Worker == nil {
					cfg.Worker = &Worker{}
				}
				cfg.Worker.Suffix = &alpha
			case "nil":
				boardEntry.WorkerSuffix = nil
				board.WorkerSuffix = nil
				if cfg.Worker != nil {
					cfg.Worker.Suffix = nil
				}
			case "empty":
				empty := ""
				boardEntry.WorkerSuffix = &empty
			}

			// Set worker_name if needed
			if tt.hasWorkerName {
				boardEntry.WorkerName = "custom"
			}

			board.Boards["board-a"] = boardEntry

			resolved, err := ResolveBoard(cfg, "test", "board-a")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPrefix, resolved.WorkerPrefix, "prefix mismatch")
			assert.Equal(t, tt.expectedWorker, resolved.Worker, "worker mismatch")
		})
	}
}
