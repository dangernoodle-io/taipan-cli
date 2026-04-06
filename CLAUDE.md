# taipan

Companion CLI for managing TaipanMiner devices.

## Module

`github.com/dangernoodle-io/taipan-cli` — Go 1.26.1

## CLI

```
taipan
  discover [flags]   # find TaipanMiner devices via mDNS
    --timeout, -t    # browse timeout in seconds (default 5)
    --json           # output as JSON
```

## Install

### From Source

```bash
go install github.com/dangernoodle-io/taipan-cli@latest
```

## Build

```
CGO_ENABLED=0 go build -o taipan ./
```

To embed a version:
```
CGO_ENABLED=0 go build -ldflags "-X github.com/dangernoodle-io/taipan-cli/internal/cli.Version=v0.1.0" -o taipan ./
```

## Packages

| Package | Purpose |
|---------|---------|
| `internal/cli/` | Cobra root + subcommand wiring |
| `internal/discover/` | mDNS browse + HTTP enrichment |
| `internal/output/` | Colored terminal output |
