# SideCar — Agent Guide

## What is it

Go app that talks to Minitela auxiliary screens over serial. Three runtime modes:
CLI (direct commands), daemon (metrics sync loop), UI (desktop webview).

## Quick commands

```
make build          # go build -o sidecar -v .
make test           # go test -v ./...
make ui             # go run . -mode ui
make daemon         # go run . -mode daemon (uses INTERVAL, DEVICE env vars)
make cli            # go run . -mode cli (uses CMD, FILE, REG, etc. env vars)
go vet ./...        # static analysis
```

## CI test order (must pass per-package)

```bash
go test -v -race -count=1 ./core/...
go test -v -race -count=1 ./metrics/...
go test -v -race -count=1 -run "^Test" .
go test -v -race -count=1 ./tests/...
```

`-race -count=1` is the CI norm.

## Key architecture

- `main.go` — entry point, parses `-mode` flag (ui|daemon|cli)
- `cli.go` — serial commands: on/off/brightness/reboot/upload/register r/w
- `daemon.go` — polls `metrics/`, writes to device registers every N seconds
- `ui.go` — webview UI, local HTTP on `127.0.0.1:8481`, embedded `ui/` via `//go:embed`
- `views.go` — 3 dashboard views (CPU+RAM / Network / Battery+Uptime)
- `core/` — serial protocol (custom frames: start=0x4148, end=0x4D49, opt CRC-16/IBM), register r/w, ACF file handling, upload with resume
- `metrics/` — Linux reads `/proc/stat`, `/proc/meminfo`, `/proc/net/dev`, `/sys/class/thermal`, ioctl for WiFi SSID
- `power/` — sleep/wake hooks via D-Bus (Linux) or Windows events
- `tests/` — black-box integration tests (import `core`, `metrics`)

## Quirks

- **Date/time encoding** is BCD-like: format as `"20250307"` then parse as hex → `0x20250307`. NOT arithmetic (`year*65536 + month*256 + day` is wrong). See `daemon.go:toBCD()`.
- **ACF checksum** property: XOR of all 32-bit LE words except bytes 4-7 and last word must equal `0xA55A5AA5`.
- **ACF test** skips if `/var/home/*/positivo/` tree is absent (requires ACF firmware dump).
- **Upload** uses MD5 for resume detection, auto-wraps image data in ACF project if it has the 8-byte header.
- **UI debug mode**: `SIDECAR_DEBUG=1` enables webview devtools.
- **Screen resolution**: 240×240, RGB565 pixel format.
- **Serial auto-connect**: probes all ports with handshake, first to respond wins.
- **Registers**: system (4, 5, 7, 2) and data (1080-2003 range). Max 16 per batch.
- **No formatter config** beyond `go fmt` / `go vet`. No linter.
- **Module**: `github.com/FreyreCorona/SideCar`, Go 1.26. Arch: linux/amd64 + windows/amd64.

## When editing

- `core/` serial protocol: frame format is `[0x41,0x48] [ctrl 2BE] [type 2BE] [content N] [crc 2BE] [0x4D,0x49]`.
- Response types = command type | 0x0040 (e.g. `CmdHandshake=0x0080` → response `0x00C0`).
- Add new register IDs to `core/tags.go`, keep in 1080-2003 range for data or 0-15 for system.
- New views go in `views.go` and the 3-view cycle in `ui.go` (`currentView = idx % 3`).
- Metrics collectors are platform-specific (`_linux.go`, `_windows.go`).
