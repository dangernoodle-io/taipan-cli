# taipan

[![Go](https://img.shields.io/github/go-mod/go-version/dangernoodle-io/taipan-cli)](https://golang.org)

A companion CLI for managing [TaipanMiner](https://github.com/dangernoodle-io/TaipanMiner) devices.

> **Maintained by AI** — This project is developed and maintained by Claude (via [@dangernoodle-io](https://github.com/dangernoodle-io)).
> If you find a bug or have a feature request, please [open an issue](https://github.com/dangernoodle-io/taipan-cli/issues) with examples so it can be addressed.

## Commands

| Command | Description |
|---------|-------------|
| `discover` | Find TaipanMiner devices on the local network via mDNS |
| `config` | Manage configuration profiles (init, get, set, list) |
| `flash` | Flash firmware and NVS configuration to a device via serial |

## Flash Command

### Pre-Flash Checks

The `flash` command performs validation checks before writing to the device:

1. **Firmware Validation** — Verifies the firmware binary contains the correct app descriptor and matches the specified board
2. **Size Check** — Ensures the binary fits in the OTA partition (1.875 MB)
3. **Device Cross-Check** — For OTA updates (via `--host`), verifies the remote device board matches `--board`. For serial flashing, validates the chip type matches the board

To skip these checks, use the `--force` flag:

```shell
taipan flash --board bitaxe-601 --force firmware.bin
```

## Install

### From Source

```shell
go install github.com/dangernoodle-io/taipan-cli@latest
```

### From GitHub Releases

Download pre-built binaries from [GitHub Releases](https://github.com/dangernoodle-io/taipan-cli/releases):
- `linux_amd64`
- `linux_arm64`
- `darwin_amd64`
- `darwin_arm64`

## Related Projects

- [TaipanMiner](https://github.com/dangernoodle-io/TaipanMiner) — Bitcoin mining firmware for ESP32 boards

## License

See LICENSE file in repository.