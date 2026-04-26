package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the entire taipan configuration.
type Config struct {
	Wifi     *Wifi                  `yaml:"wifi,omitempty"`
	Worker   *Worker                `yaml:"worker,omitempty"`
	Pool     *Pool                  `yaml:"pool,omitempty"`
	Device   *Device                `yaml:"device,omitempty"`
	Boards   map[string]*BoardEntry `yaml:"boards,omitempty"`
	Profiles map[string]*Profile    `yaml:"profiles"`
}

// Wifi represents global WiFi settings.
type Wifi struct {
	SSID     string `yaml:"ssid"`
	Password string `yaml:"password"`
}

// Worker represents global worker settings.
type Worker struct {
	Prefix *string `yaml:"prefix,omitempty"` // global worker prefix
	Suffix *string `yaml:"suffix"`           // nil = prompt, "" = no suffix
}

// Pool represents global pool settings.
type Pool struct {
	Host     string `yaml:"host,omitempty"`
	Port     uint16 `yaml:"port,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// Device represents global board-flash-time settings.
type Device struct {
	DisplayEn *bool `yaml:"display_en,omitempty"`
}

// Profile represents a named configuration profile.
type Profile struct {
	WifiSSID     string                 `yaml:"wifi_ssid,omitempty"`
	WifiPassword string                 `yaml:"wifi_password,omitempty"`
	PoolHost     string                 `yaml:"pool_host"`
	PoolPort     uint16                 `yaml:"pool_port"`
	PoolPassword string                 `yaml:"pool_password,omitempty"`
	Wallet       string                 `yaml:"wallet"`
	WorkerPrefix string                 `yaml:"worker_prefix"`
	WorkerSuffix *string                `yaml:"worker_suffix,omitempty"` // nil = prompt, "" = no suffix
	Boards       map[string]*BoardEntry `yaml:"boards"`
}

// BoardEntry represents a per-board override within a profile.
// IsDefault is set to true when the YAML value is just `true`.
type BoardEntry struct {
	IsDefault      bool    `yaml:"-"`
	PoolHost       string  `yaml:"pool_host,omitempty"`
	PoolPort       uint16  `yaml:"pool_port,omitempty"`
	PoolPassword   string  `yaml:"pool_password,omitempty"`
	Wallet         string  `yaml:"wallet,omitempty"`
	WorkerName     string  `yaml:"worker_name,omitempty"`
	WorkerPrefix   string  `yaml:"worker_prefix,omitempty"`
	WorkerSuffix   *string `yaml:"worker_suffix,omitempty"`
	DisplayEn      *bool   `yaml:"display_en,omitempty"`
}

// UnmarshalYAML handles both `board: true` and `board: {fields...}` syntax.
func (b *BoardEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		if node.Value == "true" {
			b.IsDefault = true
			return nil
		}
		return fmt.Errorf("invalid board entry: expected 'true' or mapping, got scalar %q", node.Value)
	}

	if node.Kind == yaml.MappingNode {
		// Unmarshal as a normal struct
		type boardEntryAlias BoardEntry
		var alias boardEntryAlias
		if err := node.Decode(&alias); err != nil {
			return err
		}
		*b = BoardEntry(alias)
		return nil
	}

	return fmt.Errorf("invalid board entry: expected 'true' or mapping, got %v", node.Kind)
}

// MarshalYAML handles marshaling: if IsDefault=true, marshal as scalar "true".
func (b *BoardEntry) MarshalYAML() (interface{}, error) {
	if b.IsDefault {
		return "true", nil
	}
	type boardEntryAlias BoardEntry
	return boardEntryAlias(*b), nil
}

// DefaultPath returns the default configuration file path: ~/.config/taipan/config.yml
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "taipan", "config.yml"), nil
}

// Load reads and unmarshals a YAML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	return cfg, nil
}

// Save marshals and writes a configuration to a YAML file with parent directories created.
func Save(path string, cfg *Config) error {
	// Create parent directories with 0755 permissions
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create parent directories: %w", err)
	}

	// Marshal to YAML with 2-space indent
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}
	_ = enc.Close()

	// Write with 0644 permissions
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}

	return nil
}

// GetProfile returns a profile by name, or an error if not found.
func (c *Config) GetProfile(name string) (*Profile, error) {
	if c.Profiles == nil {
		return nil, fmt.Errorf("no profiles found in configuration")
	}

	profile, ok := c.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}

	return profile, nil
}
