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
	PoolPassword string
	Wallet       string
	Worker       string // resolved from worker_name or prefix+suffix
	WorkerPrefix string // resolved prefix (for re-assembly when suffix is prompted later)
	DisplayEn    bool   // default true if unresolved/nil
}

// ResolveBoard resolves the configuration for a specific board within a profile.
// Strict enrollment: board MUST exist in profile.Boards.
// Resolution chain: profile.board → profile → cfg.boards[board] → cfg.<global>
// Wallet stops at profile (no global). WorkerName stays board-local (verbatim wins over prefix/suffix).
// PoolPassword defaults to "x" if entirely unresolved.
// DisplayEn defaults to true if nil/unresolved.
func ResolveBoard(cfg *Config, profileName, board string) (*ResolvedConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	if profile == nil {
		return nil, fmt.Errorf("profile %q is nil", profileName)
	}

	// Strict enrollment: board MUST exist in profile.Boards
	boardEntry, exists := profile.Boards[board]
	if !exists {
		return nil, fmt.Errorf("board %q not found in profile", board)
	}

	resolved := &ResolvedConfig{
		DisplayEn: true, // default
	}

	// WiFi SSID: profile -> global
	if profile.WifiSSID != "" {
		resolved.WifiSSID = profile.WifiSSID
	} else if cfg.Wifi != nil {
		resolved.WifiSSID = cfg.Wifi.SSID
	}

	// WiFi Password: profile -> global
	if profile.WifiPassword != "" {
		resolved.WifiPassword = profile.WifiPassword
	} else if cfg.Wifi != nil {
		resolved.WifiPassword = cfg.Wifi.Password
	}

	// PoolHost: profile.board -> profile -> cfg.boards[board] -> cfg.pool
	if boardEntry.PoolHost != "" {
		resolved.PoolHost = boardEntry.PoolHost
	} else if profile.PoolHost != "" {
		resolved.PoolHost = profile.PoolHost
	} else if cfg.Boards != nil {
		if globalBoard, ok := cfg.Boards[board]; ok && globalBoard.PoolHost != "" {
			resolved.PoolHost = globalBoard.PoolHost
		} else if cfg.Pool != nil {
			resolved.PoolHost = cfg.Pool.Host
		}
	} else if cfg.Pool != nil {
		resolved.PoolHost = cfg.Pool.Host
	}

	// PoolPort: same chain (0 allowed)
	if boardEntry.PoolPort != 0 {
		resolved.PoolPort = boardEntry.PoolPort
	} else if profile.PoolPort != 0 {
		resolved.PoolPort = profile.PoolPort
	} else if cfg.Boards != nil {
		if globalBoard, ok := cfg.Boards[board]; ok && globalBoard.PoolPort != 0 {
			resolved.PoolPort = globalBoard.PoolPort
		} else if cfg.Pool != nil {
			resolved.PoolPort = cfg.Pool.Port
		}
	} else if cfg.Pool != nil {
		resolved.PoolPort = cfg.Pool.Port
	}

	// PoolPassword: same chain; default to "x" if entirely unset
	if boardEntry.PoolPassword != "" {
		resolved.PoolPassword = boardEntry.PoolPassword
	} else if profile.PoolPassword != "" {
		resolved.PoolPassword = profile.PoolPassword
	} else if cfg.Boards != nil {
		if globalBoard, ok := cfg.Boards[board]; ok && globalBoard.PoolPassword != "" {
			resolved.PoolPassword = globalBoard.PoolPassword
		} else if cfg.Pool != nil && cfg.Pool.Password != "" {
			resolved.PoolPassword = cfg.Pool.Password
		} else {
			resolved.PoolPassword = "x"
		}
	} else if cfg.Pool != nil && cfg.Pool.Password != "" {
		resolved.PoolPassword = cfg.Pool.Password
	} else {
		resolved.PoolPassword = "x"
	}

	// Wallet: board -> profile (no global)
	if boardEntry.Wallet != "" {
		resolved.Wallet = boardEntry.Wallet
	} else {
		resolved.Wallet = profile.Wallet
	}

	// Resolve worker prefix: board -> profile -> cfg.boards[board] -> cfg.worker
	prefix := ""
	if boardEntry.WorkerPrefix != "" {
		prefix = boardEntry.WorkerPrefix
	} else if profile.WorkerPrefix != "" {
		prefix = profile.WorkerPrefix
	} else if cfg.Boards != nil {
		if globalBoard, ok := cfg.Boards[board]; ok && globalBoard.WorkerPrefix != "" {
			prefix = globalBoard.WorkerPrefix
		} else if cfg.Worker != nil && cfg.Worker.Prefix != nil {
			prefix = *cfg.Worker.Prefix
		}
	} else if cfg.Worker != nil && cfg.Worker.Prefix != nil {
		prefix = *cfg.Worker.Prefix
	}

	// Store resolved prefix
	resolved.WorkerPrefix = prefix

	// Resolve worker name
	if boardEntry.WorkerName != "" {
		// If board has explicit worker_name, use it as-is
		resolved.Worker = boardEntry.WorkerName
	} else {
		// Assemble from prefix+suffix with precedence
		// Resolve suffix: board -> profile -> cfg.worker
		var suffix *string
		if boardEntry.WorkerSuffix != nil {
			suffix = boardEntry.WorkerSuffix
		} else if profile.WorkerSuffix != nil {
			suffix = profile.WorkerSuffix
		} else if cfg.Worker != nil {
			suffix = cfg.Worker.Suffix
		}

		// Assemble worker from prefix+suffix
		if suffix == nil {
			// nil suffix means we'll prompt later
			resolved.Worker = ""
		} else if *suffix == "" {
			// empty string suffix means no suffix, just prefix
			resolved.Worker = prefix
		} else {
			// normal case: prefix + "-" + suffix
			resolved.Worker = prefix + "-" + *suffix
		}
	}

	// DisplayEn: board -> cfg.boards[board] -> cfg.device -> default true
	if boardEntry.DisplayEn != nil {
		resolved.DisplayEn = *boardEntry.DisplayEn
	} else if cfg.Boards != nil {
		if globalBoard, ok := cfg.Boards[board]; ok && globalBoard.DisplayEn != nil {
			resolved.DisplayEn = *globalBoard.DisplayEn
		} else if cfg.Device != nil && cfg.Device.DisplayEn != nil {
			resolved.DisplayEn = *cfg.Device.DisplayEn
		}
		// else defaults to true (already set)
	} else if cfg.Device != nil && cfg.Device.DisplayEn != nil {
		resolved.DisplayEn = *cfg.Device.DisplayEn
	}
	// else defaults to true (already set)

	return resolved, nil
}

// Get retrieves a value using dot-path notation.
// Leading "wifi" or "worker" operate on cfg level (ignore profileName).
// Otherwise, look up cfg.Profiles[profileName] and dispatch.
// Examples: "wifi.ssid", "worker.suffix", "pool_host", "boards.tdongle-s3.pool_port"
func Get(cfg *Config, profileName, key string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is nil")
	}

	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("empty key")
	}

	// Config-level keys
	switch parts[0] {
	case "wifi":
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Wifi == nil {
			return "", nil
		}
		switch parts[1] {
		case "ssid":
			return cfg.Wifi.SSID, nil
		case "password":
			return cfg.Wifi.Password, nil
		default:
			return "", fmt.Errorf("invalid wifi field: %q", parts[1])
		}

	case "worker":
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		switch parts[1] {
		case "prefix":
			if cfg.Worker == nil || cfg.Worker.Prefix == nil {
				return "", nil
			}
			return *cfg.Worker.Prefix, nil
		case "suffix":
			if cfg.Worker == nil || cfg.Worker.Suffix == nil {
				return "", nil
			}
			return *cfg.Worker.Suffix, nil
		default:
			return "", fmt.Errorf("invalid worker field: %q", parts[1])
		}

	case "pool":
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Pool == nil {
			return "", nil
		}
		switch parts[1] {
		case "host":
			return cfg.Pool.Host, nil
		case "port":
			return fmt.Sprintf("%d", cfg.Pool.Port), nil
		case "password":
			return cfg.Pool.Password, nil
		default:
			return "", fmt.Errorf("invalid pool field: %q", parts[1])
		}

	case "device":
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Device == nil {
			return "", nil
		}
		switch parts[1] {
		case "display_en":
			if cfg.Device.DisplayEn == nil {
				return "", nil
			}
			return fmt.Sprintf("%v", *cfg.Device.DisplayEn), nil
		default:
			return "", fmt.Errorf("invalid device field: %q", parts[1])
		}

	case "boards":
		// Global boards routing: boards.<name>.<field> with no profile
		if profileName == "" {
			if len(parts) < 2 {
				return "", fmt.Errorf("invalid key: %q", key)
			}
			boardName := parts[1]
			boardEntry, ok := cfg.Boards[boardName]
			if !ok {
				return "", fmt.Errorf("board %q not found in global boards", boardName)
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
			case "pool_password":
				return boardEntry.PoolPassword, nil
			case "wallet":
				return boardEntry.Wallet, nil
			case "worker_name":
				return boardEntry.WorkerName, nil
			case "worker_prefix":
				return boardEntry.WorkerPrefix, nil
			case "worker_suffix":
				if boardEntry.WorkerSuffix == nil {
					return "", nil
				}
				return *boardEntry.WorkerSuffix, nil
			case "display_en":
				if boardEntry.DisplayEn == nil {
					return "", nil
				}
				return fmt.Sprintf("%v", *boardEntry.DisplayEn), nil
			default:
				return "", fmt.Errorf("invalid board field: %q", field)
			}
		}
		// else: fall through to profile-level boards (below)
	}

	// Profile-level keys
	if profileName == "" {
		return "", fmt.Errorf("profile required for key %q", key)
	}

	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return "", fmt.Errorf("profile %q not found", profileName)
	}
	if profile == nil {
		return "", fmt.Errorf("profile %q is nil", profileName)
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

	case "pool_password":
		if len(parts) != 1 {
			return "", fmt.Errorf("invalid key: %q", key)
		}
		return profile.PoolPassword, nil

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
		case "worker_prefix":
			return boardEntry.WorkerPrefix, nil
		case "worker_suffix":
			if boardEntry.WorkerSuffix == nil {
				return "", nil
			}
			return *boardEntry.WorkerSuffix, nil
		case "pool_password":
			return boardEntry.PoolPassword, nil
		case "display_en":
			if boardEntry.DisplayEn == nil {
				return "", nil
			}
			return fmt.Sprintf("%v", *boardEntry.DisplayEn), nil
		default:
			return "", fmt.Errorf("invalid board field: %q", field)
		}

	default:
		return "", fmt.Errorf("invalid key: %q", key)
	}
}

// Set sets a value using dot-path notation.
// Leading "wifi" or "worker" operate on cfg level (create if nil).
// Otherwise, look up cfg.Profiles[profileName] and dispatch.
func Set(cfg *Config, profileName, key string, value string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}

	// Config-level keys
	switch parts[0] {
	case "wifi":
		if len(parts) != 2 {
			return fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Wifi == nil {
			cfg.Wifi = &Wifi{}
		}
		switch parts[1] {
		case "ssid":
			cfg.Wifi.SSID = value
			return nil
		case "password":
			cfg.Wifi.Password = value
			return nil
		default:
			return fmt.Errorf("invalid wifi field: %q", parts[1])
		}

	case "worker":
		if len(parts) != 2 {
			return fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Worker == nil {
			cfg.Worker = &Worker{}
		}
		switch parts[1] {
		case "prefix":
			cfg.Worker.Prefix = &value
			return nil
		case "suffix":
			cfg.Worker.Suffix = &value
			return nil
		default:
			return fmt.Errorf("invalid worker field: %q", parts[1])
		}

	case "pool":
		if len(parts) != 2 {
			return fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Pool == nil {
			cfg.Pool = &Pool{}
		}
		switch parts[1] {
		case "host":
			cfg.Pool.Host = value
			return nil
		case "port":
			port, err := strconv.ParseUint(value, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid pool port: %w", err)
			}
			cfg.Pool.Port = uint16(port)
			return nil
		case "password":
			cfg.Pool.Password = value
			return nil
		default:
			return fmt.Errorf("invalid pool field: %q", parts[1])
		}

	case "device":
		if len(parts) != 2 {
			return fmt.Errorf("invalid key: %q", key)
		}
		if cfg.Device == nil {
			cfg.Device = &Device{}
		}
		switch parts[1] {
		case "display_en":
			// Parse "true"/"false" to bool
			switch value {
			case "true":
				v := true
				cfg.Device.DisplayEn = &v
				return nil
			case "false":
				v := false
				cfg.Device.DisplayEn = &v
				return nil
			default:
				return fmt.Errorf("invalid display_en value: must be 'true' or 'false', got %q", value)
			}
		default:
			return fmt.Errorf("invalid device field: %q", parts[1])
		}

	case "boards":
		// Global boards routing: boards.<name>.<field> with no profile
		if profileName == "" {
			if len(parts) < 3 {
				return fmt.Errorf("invalid key: %q", key)
			}
			boardName := parts[1]
			field := parts[2]

			if cfg.Boards == nil {
				cfg.Boards = make(map[string]*BoardEntry)
			}

			boardEntry, ok := cfg.Boards[boardName]
			if !ok {
				boardEntry = &BoardEntry{}
				cfg.Boards[boardName] = boardEntry
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
			case "pool_password":
				boardEntry.PoolPassword = value
				return nil
			case "wallet":
				boardEntry.Wallet = value
				return nil
			case "worker_name":
				boardEntry.WorkerName = value
				return nil
			case "worker_prefix":
				boardEntry.WorkerPrefix = value
				return nil
			case "worker_suffix":
				boardEntry.WorkerSuffix = &value
				return nil
			case "display_en":
				// Parse "true"/"false" to bool
				switch value {
				case "true":
					v := true
					boardEntry.DisplayEn = &v
					return nil
				case "false":
					v := false
					boardEntry.DisplayEn = &v
					return nil
				default:
					return fmt.Errorf("invalid display_en value: must be 'true' or 'false', got %q", value)
				}
			default:
				return fmt.Errorf("invalid board field: %q", field)
			}
		}
		// else: fall through to profile-level boards (below)
	}

	// Profile-level keys
	if profileName == "" {
		return fmt.Errorf("profile required for key %q", key)
	}

	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}
	if profile == nil {
		return fmt.Errorf("profile %q is nil", profileName)
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

	case "pool_password":
		if len(parts) != 1 {
			return fmt.Errorf("invalid key: %q", key)
		}
		profile.PoolPassword = value
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
		case "worker_prefix":
			boardEntry.WorkerPrefix = value
			return nil
		case "worker_suffix":
			boardEntry.WorkerSuffix = &value
			return nil
		case "pool_password":
			boardEntry.PoolPassword = value
			return nil
		case "display_en":
			// Parse "true"/"false" to bool
			switch value {
			case "true":
				v := true
				boardEntry.DisplayEn = &v
				return nil
			case "false":
				v := false
				boardEntry.DisplayEn = &v
				return nil
			default:
				return fmt.Errorf("invalid display_en value: must be 'true' or 'false', got %q", value)
			}
		default:
			return fmt.Errorf("invalid board field: %q", field)
		}

	default:
		return fmt.Errorf("invalid key: %q", key)
	}
}
