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

# CLI commands (auto-detects CLI mode when -cmd is used)
sidecar -cmd on                    # turn screen on
sidecar -cmd off                   # turn screen off
sidecar -cmd upload -file X.acf    # upload ACF file
sidecar -cmd generate-acf -zip file.zip  # generate ACF from zip
sidecar -cmd upload -zip file.zip  # generate + upload in one step
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
- `core/` — serial protocol (custom frames: start=0x4148, end=0x4D49, opt CRC-16/IBM), register r/w, ACF file handling, upload with resume, ACF generation via Wine
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
- **Module**: `github.com/FreyreCorona/SideCar`, Go 1.27. Arch: linux/amd64 + windows/amd64.

## When editing

- `core/` serial protocol: frame format is `[0x41,0x48] [ctrl 2BE] [type 2BE] [content N] [crc 2BE] [0x4D,0x49]`.
- Response types = command type | 0x0040 (e.g. `CmdHandshake=0x0080` → response `0x00C0`).
- Add new register IDs to `core/tags.go`, keep in 1080-2003 range for data or 0-15 for system.
- New views go in `views.go` and the 3-view cycle in `ui.go` (`currentView = idx % 3`).
- Metrics collectors are platform-specific (`_linux.go`, `_windows.go`).

## Hardware test findings (Minitela device)

### Post-upload register writes — FIXED
- After `UploadFile` + `DownloadComplete`, device reboots (USB CDC disconnects/re-enumerates, port may change ACM0→ACM1→ACM0).
- **Two bugs were fixed:**
  1. **CRC zero-tolerance** (`core/protocol.go`): device sends SetRegisterResp with CRC bit set but CRC value `0x0000`. Fix: treat `crcField==0x0000` as "CRC not computed" and accept.
  2. **functionCode=2 rejection** (`core/register.go`): device sends functionCode=2 in write ACKs. Fix: accept any functionCode, return empty map (matches Minipanel behavior which only checks commandType).
- Images still not displayed on screen — ACF format mismatch (our `ImageToACF` vs. proprietary `AHMISimGenDemo`).

### Post-upload page switching — requires reconnect
- After upload+reboot, device enumerates on a new (or same) USB serial port.
- The `cmd/test/main.go` tests the full flow: upload → wait → reconnect → handshake → API register write.
- The Minipanel app's `sendCommandUntilResponse` retries for 60s total with 1s retry interval. Our `sendWithRetry` is 3 retries × 1s timeout (~4s total). May need longer for post-reboot.

### Debug tools available
- `SERIAL_DEBUG=1` env var → logs every serial read (hex dump to stderr).
- `cmd/test/main.go` → standalone test program for upload + API-level register diagnostics.
- `core/device.go` → `WriteRaw`, `ReadRaw`, `SerialPort()` for raw access.
- `core/serial.go` → `Drain()` clears OS + internal buffer (100ms).

### Device protocol notes
- Flood of `DownloadDataResp` (0x00C2) frames during upload — normal behavior, device acknowledges each chunk.
- `Drain()` in `sendWithRetry` (100ms) can't keep up with flood during upload; works fine in normal operation.
- Handshake response: CRC disabled (`0x0006`), always succeeds.
- Register responses after upload: CRC enabled (`0x8008`) with `0x0000` CRC — likely firmware bug or state-dependent behavior.

## ACF generation pipeline (WORKING)

The Minitela app uses `AHMISimGenDemo_og.exe` (Windows PE32) to compile `file.zip` → ACF. We can run it via Wine in a distrobox container.

### Setup (one-time)
```bash
# Create distrobox with Wine
distrobox create --name winebox --image ubuntu:24.04
distrobox enter winebox -- sudo dpkg --add-architecture i386
distrobox enter winebox -- sudo apt-get update
distrobox enter winebox -- sudo apt-get install -y wine wine32:i386 winetricks cabextract p7zip-full imagemagick gcc-mingw-w64-i686
```

### Working directory layout
All files live in `~/ahmi-work/Gen/` inside the distrobox (shared home with host).

**Source of real binaries**: extracted AppImage at `/tmp/squashfs-root/resources/IDE_utils/Gen/`
- Copy the entire `Gen/` dir into `~/ahmi-work/Gen/` (exe, DLLs, configInfo/, font/, VersionInfomation/, zip64/)
- The AppImage's `zip64/7z.exe` is 64-bit — replace with the 32-bit `7za.exe` from `7z2301-extra.7z` (downloadable from 7-zip.org):
  ```
  curl -sL "https://www.7-zip.org/a/7z2301-extra.7z" -o /tmp/7z-extra.7z
  7z x /tmp/7z-extra.7z -o/tmp/7z-extracted/ -y
  cp /tmp/7z-extracted/7za.exe ~/ahmi-work/Gen/zip64/7z.exe
  ```
- Missing DLLs from Gitee are truncated (login required for large files) — the AppImage has all working copies.

### Generate ACF from file.zip
```bash
distrobox enter winebox -- bash << 'SCRIPT'
cd ~/ahmi-work/Gen
rm -rf json/
# file.zip must be in CWD (Gen/)
echo "13" | WINEDEBUG=-all wine AHMISimGenDemo_og.exe -f file.zip -m 2 -c 0 -e 0 -d 1 -o ../ACF 2>&1
# Output: ../ACF/Texture.acf and ../ACF/ConfigData&Texture.acf
SCRIPT
```

### Replace images in file.zip
Use Python to modify the zip — replace GIF/PNG files by name:
```python
import zipfile, shutil
zippath = "/home/freyre/ahmi-work/file.zip"
with zipfile.ZipFile(zippath, 'r') as zin:
    with zipfile.ZipFile(tmpzip, 'w', zipfile.ZIP_DEFLATED) as zout:
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == "target_image.gif":
                data = open("new_image.gif", "rb").read()
            zout.writestr(item, data)
shutil.move(tmpzip, zippath)
```

### Upload and display
```bash
cp ~/ahmi-work/ACF/Texture.acf ~/Development/FreyreCorona/SideCar/cmd/test/generated.acf
./sidecar -mode cli -cmd upload -file cmd/test/generated.acf -type texture
sleep 10
./sidecar -mode cli -cmd reboot
sleep 15
./sidecar -mode cli -cmd show-page -page 5
```

### Key facts
- **exe args**: `-f file.zip -m 2 -c 0 -e <platformCode> -d <dither> -o <outdir>` (platformCode: 0=GC9002, 1=GC9003, 2=GC9005)
- **exe expects** `13` on stdin when prompted "Press any key to continue..."
- **configInfo/products.json**: must list the target device (GC9002=240x240 not in default — use the one from the AppImage)
- **configInfo/all.cfg**: hardware string `HWX0104.00EM0304.01NOR256D365536...`
- **configInfo/scr.cfg**: screen resolution (4 bytes, binary)
- **configInfo/pro.cfg**: parsed hardware config fields
- **VersionInfomation/YTH_Version.Info**: version string `1.10.19_build_2024.09.20_GIFTest`
- **font/**: contains `FontsView_num.exe`, `FontsView_string.exe`, `NUM.txt`, `font_alpha2.dat`, `font_alpha4.dat`
- **zip64/**: contains `7z.exe` (must be 32-bit PE32), `7z.dll`, `D3DX9_41.dll`, `texconv.exe`
- **Output**: `Texture.acf` (texture only) and `ConfigData&Texture.acf` (full ACF with config + texture)
- Generated ACF magic is `0x4000` (vs reference `0x8000`) — both work on device
- **file.zip structure**: `data.json` (project definition with pages, widgets, tags, resources) + PNG/GIF resources + font PNGs
- **Pages**: 0-indexed in data.json. Pages 4-6 are GIF pages (animated). Pages 0-3 have static UI with widgets.
- **GIF frames**: each GIF is decomposed into individual PNGs (`name_0.png`, `name_1.png`, etc) by the exe
- **After upload+reboot**: device needs ~10s to reload, then use `show-page` to navigate

### `file.zip` contents (from device deb)
- `data.json` (393KB): complete AHMI project definition — pageList (10 pages), resourceList (62 items), tagList (57 tags)
- GIFs: `1i1h1e37393671471.gif` (page 4, 30 frames), `1h1k1e37393671464.gif` (page 5, 30 frames), `1h1m1e37393671466.gif` (page 6, 44 frames)
- Font PNGs: Inconsolata at various sizes (15, 19, 22, 30, 40px)
- Background images: `r-0-0.png`, `r-1-0.png`, `r-2-0.png`, etc.

### Troubleshooting
- "products config文件不能打开": `configInfo/products.json` missing or doesn't contain target device
- "no yth version file": `VersionInfomation/YTH_Version.Info` missing
- "connot open filevideo.bmp": `video.bmp` missing in CWD (non-fatal, can be empty)
- Font error at FontClass.cpp line 140: `font/` directory missing or `font_alpha4.dat` corrupted
- ACF too large / display issues: check that GIFs are valid and not corrupted during zip replacement
- Device shows old content after upload: must reboot (`show-page` alone doesn't reload ACF from flash)

## Uncommitted changes
- `core/protocol.go`: CRC zero-tolerance fix (treats 0x0000 CRC as valid).
- `core/register.go`: functionCode=2 acceptance in `parseNumRegResponse` (returns empty map instead of error).
- `cmd/test/main.go`: full API-level test after upload+reconnect (not just raw writes).

## Reference files (positivo/)
- `minipanel-src/src/utils/command.js` — protocol implementation (Node.js). Reference for frame format, CRC, command types.
- `minipanel-src/src/utils/messageProcessor.js` — register read/write (numeric and string). Key: `sendNumTagData16` sends `0x80 | (count-1)` header, `sendStringTagData` sends `0xD0` header.
- `minipanel-src/src/utils/port.js` — serial port manager with retry logic. Uses `sendCommandUntilResponse` which retries for 60s.
- `minipanel-src/src/utils/acfGenerator.js` — calls `AHMISimGenDemo_og.exe` to generate ACF from zip. Not reimplementable (Windows binary).
- `minipanel-src/src/utils/tagUtils.js` — register ID map (CPU_Usage=1080, Battery=1082, etc). `systemTagNameMap`: Date=4, Time=5, Backlight=7, CurrentPage=2.
- `acf-samples/` — reference ACF files from device firmware (Texture.acf template, Texture1-21 with images, filevector_0-4 raw blocks).
- `config/products.json` — device config: GC9002 (240x240), GC9003 (1280x960), GC9001 (800x480).

## ACF format (from analysis)
- Template `Texture.acf` (851976 bytes) = header(0x4000) + 12 blocks of 0x10000 + image data + footer.
- Image ACFs (`Texture1.acf` etc, 1.5-2.3MB) = same structure but with actual RGB565 image data in blocks.
- `filevector_0` = 16KB header block (same as ACF header). `filevector_1` = 770048 bytes (image data?). `filevector_3` = 829448 bytes (full ACF?).
- ACF generation is done by `AHMISimGenDemo_og.exe` (Windows binary, uses OpenCV). Cannot be reimplemented easily.
- The zip file (`file.zip`) contains the project definition (JSON) + resources (PNG images). The exe converts this to ACF format.

## SetRegister protocol (from Minipanel source)
- **Numeric write**: content = `[0x80 | (count-1)] [regId 2BE] [value 4BE] ...`
- **String write**: content = `[0xD0] [regId 2BE] [len 2BE] [string bytes]`
- **Numeric read**: content = `[0xC0 | (count-1)] [regId 2BE] ...`
- **String read**: content = `[0xE0] [regId 2BE] [len 2BE]`
- **Response**: content = `[reserved:1 | funcCode:3 | regNum:1] [data]`
  - funcCode=0 → numeric data, funcCode=3 → string data
  - Minipanel app does NOT validate funcCode at protocol level (only checks commandType==0x00D0)
  - funcCode=2 seen in device write ACKs after upload — accepted (returns empty map)
