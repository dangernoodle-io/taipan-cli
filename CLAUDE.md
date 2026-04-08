# taipan

Companion CLI for managing TaipanMiner devices.

## Module

`github.com/dangernoodle-io/taipan-cli` — Go 1.26.1

## CLI

```
taipan
  discover [flags]           # find TaipanMiner devices via mDNS
    --timeout, -t            # browse timeout in seconds (default 5)
    --json                   # output as JSON
  config                     # manage configuration profiles
    init [--profile NAME]    # create a new profile interactively
    get KEY [--profile NAME] # get a config value (dot-path keys)
    set KEY VALUE [--profile NAME] # set a config value
    list [--profile NAME]    # print profile as YAML
  flash [firmware.bin]       # flash firmware + NVS config to a device
    --board, -b              # board type (required)
    --port, -p               # serial port
    --profile                # config profile (default: "default")
    --latest                 # pull latest release from GitHub
  logs [flags]               # stream logs from TaipanMiner devices
    --host HOSTNAME          # target specific device(s) (repeatable)
    --all                    # stream logs from all discovered devices
    --timeout, -t            # discovery timeout in seconds (default 5)
  update                     # trigger OTA firmware update on devices
    --host HOSTNAME          # target specific device(s) (repeatable)
    --all                    # update all discovered devices
    --timeout, -t            # discovery timeout in seconds (default 5)
```

## Install

### From Source

```bash
go install github.com/dangernoodle-io/taipan-cli@latest
```

## Build

```
go build -o taipan ./
```

To embed a version:
```
go build -ldflags "-X github.com/dangernoodle-io/taipan-cli/internal/cli.Version=v0.1.0" -o taipan ./
```

## Config

Profile-based configuration at `~/.config/taipan/config.yml`. Profiles define WiFi, pool, wallet, and worker settings with per-board overrides.

## Packages

| Package | Purpose |
|---------|---------|
| `internal/cli/` | Cobra root + subcommand wiring |
| `internal/config/` | Profile config types, YAML load/save, board resolution |
| `internal/discover/` | mDNS browse + HTTP enrichment |
| `internal/flash/` | NVS binary gen, GitHub release download, serial flash orchestration |
| `internal/ota/` | OTA update client (check, trigger, poll status) |
| `internal/output/` | Colored terminal output |
