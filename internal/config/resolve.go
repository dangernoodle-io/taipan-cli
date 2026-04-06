package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ResolvedConfig contains the resolved configuration for a specific board.
type ResolvedConfig struct {
	WifiSSID     string
	WifiPassword string
	PoolHost     string
	PoolPort     uint16
	Wallet       string
	Worker       string // resolved from worker_name or prefix+suffix
}

// ResolveBoard resolves the configuration for a specific board within a profile.
// It merges profile defaults with board-specific overrides.
func ResolveBoard(profile *Profile, board string) (*ResolvedConfig, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile is nil")
	}

	// Check if board exists
	boardEntry, exists := profile.Boards[board]
	if !exists {
		return nil, fmt.Errorf("board %q not found in profile", board)
	}

	resolved := &ResolvedConfig{}

	// WiFi settings come only from profile level
	resolved.WifiSSID = profile.WifiSSID
	resolved.WifiPassword = profile.WifiPassword

	// Resolve pool_host: profile default, board overrides if non-empty
	resolved.PoolHost = profile.PoolHost
	if boardEntry.PoolHost != "" {
		resolved.PoolHost = boardEntry.PoolHost
	}

	// Resolve pool_port: profile default, board overrides if non-zero
	resolved.PoolPort = profile.PoolPort
	if boardEntry.PoolPort != 0 {
		resolved.PoolPort = boardEntry.PoolPort
	}

	// Resolve wallet: profile default, board overrides if non-empty
	resolved.Wallet = profile.Wallet
	if boardEntry.Wallet != "" {
		resolved.Wallet = boardEntry.Wallet
	}

	// Resolve worker name
	if boardEntry.WorkerName != "" {
		// If board has explicit worker_name, use it as-is
		resolved.Worker = boardEntry.WorkerName
	} else {
		// Resolve worker_suffix: board can override profile-level suffix
		var suffix *string
		if boardEntry.WorkerSuffix != nil {
			suffix = boardEntry.WorkerSuffix
		} else {
			suffix = profile.WorkerSuffix
		}

		// Assemble worker from prefix+suffix
		if suffix == nil {
			// nil suffix means we'll prompt later
			resolved.Worker = ""
		} else if *suffix == "" {
			// empty string suffix means no suffix, just prefix
			resolved.Worker = profile.WorkerPrefix
		} else {
			// normal case: prefix + "-" + suffix
			resolved.Worker = profile.WorkerPrefix + "-" + *suffix
		}
	}

	return resolved, nil
}

// Get retrieves a value from the profile using dot-path notation.
// Examples: "wifi_ssid", "boards.tdongle-s3.pool_port"
func Get(profile *Profile, key string) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("profile is nil")
	}

	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("empty key")
	}

	switch parts[0] {
	case "wifi_ssid":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.WifiSSID, nil

	case "wifi_password":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.WifiPassword, nil

	case "pool_host":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.PoolHost, nil

	case "pool_port":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return fmt.Sprintf("%d", profile.PoolPort), nil

	case "wallet":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.Wallet, nil

	case "worker_prefix":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.WorkerPrefix, nil

	case "worker_suffix":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		if profile.WorkerSuffix == nil {
			return "", nil
		}
		return *profile.WorkerSuffix, nil

	case "boards":
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		boardName := parts[1]
		boardEntry, ok := profile.Boards[boardName]
		if !ok {
			return "", fmt.Errorf("board %q not found", boardName)
		}

		if len(parts) == 2 {
			// Just asking for the board existence
			if boardEntry.IsDefault {
				return "true", nil
			}
			return "map", nil
		}

		// Get field within board
		field := parts[2]
		switch field {
		case "pool_host":
			return boardEntry.PoolHost, nil
		case "pool_port":
			return fmt.Sprintf("%d", boardEntry.PoolPort), nil
		case "wallet":
			return boardEntry.Wallet, nil
		case "worker_name":
			return boardEntry.WorkerName, nil
		case "worker_suffix":
			if boardEntry.WorkerSuffix == nil {
				return "", nil
			}
			return *boardEntry.WorkerSuffix, nil
		default:
			return "", fmt.Errorf("invalid board field: %q", field)
		}

	default:
		return "", fmt.Errorf("invalid key: %q", key)
	}
}

// Set sets a value in the profile using dot-path notation.
// Parses numeric types as needed.
func Set(profile *Profile, key string, value string) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}

	switch parts[0] {
	case "wifi_ssid":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.WifiSSID = value
		return nil

	case "wifi_password":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.WifiPassword = value
		return nil

	case "pool_host":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.PoolHost = value
		return nil

	case "pool_port":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid pool_port: %w", err)
		}
		profile.PoolPort = uint16(port)
		return nil

	case "wallet":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.Wallet = value
		return nil

	case "worker_prefix":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.WorkerPrefix = value
		return nil

	case "worker_suffix":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.WorkerSuffix = &value
		return nil

	case "boards":
		if len(parts) < 3 {
			return fmt.Errorf("invalid key: %q", key)
		}
		boardName := parts[1]
		field := parts[2]

		if profile.Boards == nil {
			profile.Boards = make(map[string]*BoardEntry)
		}

		boardEntry, ok := profile.Boards[boardName]
		if !ok {
			boardEntry = &BoardEntry{}
			profile.Boards[boardName] = boardEntry
		}

		switch field {
		case "pool_host":
			boardEntry.PoolHost = value
			return nil
		case "pool_port":
			port, err := strconv.ParseUint(value, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid pool_port: %w", err)
			}
			boardEntry.PoolPort = uint16(port)
			return nil
		case "wallet":
			boardEntry.Wallet = value
			return nil
		case "worker_name":
			boardEntry.WorkerName = value
			return nil
		case "worker_suffix":
			boardEntry.WorkerSuffix = &value
			return nil
		default:
			return fmt.Errorf("invalid board field: %q", field)
		}

	default:
		return fmt.Errorf("invalid key: %q", key)
	}
}
