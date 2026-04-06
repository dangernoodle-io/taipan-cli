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