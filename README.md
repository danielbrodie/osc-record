# osc-record

OSC-triggered video capture for live production. Send an OSC message from QLab, Disguise, a lighting console, or any OSC source — recording starts on every configured device simultaneously. Send another — everything stops and files are saved.

Built for theatrical environments where recording needs to be hands-free, show-control-driven, and rock solid.

---

## What it does

- **OSC-triggered**: any OSC-capable show control system can start and stop recording
- **Multi-device**: one trigger fires all devices simultaneously — decklink cards, webcams, or mixed
- **Full TUI**: live signal status, VU meters, OSC monitor, clip list, disk usage — all in terminal
- **Automation-friendly**: `--no-tui` flag and non-TTY auto-detection for headless/cron use
- **Blackmagic DeckLink first**: decklink mode auto-selects if supported; avfoundation/dshow as fallback
- **HTTP status API**: `/status`, `/clips`, `/health` for integration with other systems
- **Post-show manifest**: JSON log of every clip from the session

---

## Install

### macOS (Homebrew)

```sh
brew tap danielbrodie/tap
brew install osc-record
```

This installs `osc-record` and a pre-built ffmpeg with DeckLink support. You also need [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) drivers installed for decklink capture.

### Manual (all platforms)

Download the latest binary from [Releases](https://github.com/danielbrodie/osc-record/releases) and put it in your `$PATH`.

You need ffmpeg in your `$PATH` (or set `ffmpeg.path` in config). For Blackmagic devices, ffmpeg must be built with `--enable-decklink`.

### Windows

Download `osc-record_windows_amd64.zip` from [Releases](https://github.com/danielbrodie/osc-record/releases). Requires ffmpeg with DeckLink support and Blackmagic Desktop Video drivers.

---

## Quick Start

### 1. Run setup

```sh
osc-record setup
```

The setup wizard walks you through: picking your capture device, configuring your record/stop OSC addresses (it listens for a real OSC message so you just fire the cue), setting output directory and filename prefix. Config is saved automatically.

For headless/automation environments:

```sh
osc-record setup --no-tui
```

### 2. Start recording

```sh
osc-record run
```

The TUI launches showing signal lock status, VU meters, OSC monitor, and clip list. Send your record OSC message — recording starts on all devices. Send the stop message — files are saved.

### 3. Non-interactive / headless

```sh
osc-record run --no-tui
```

Runs as a plain daemon. Prints status lines to stdout. Works in systemd, launchd, tmux sessions, cron-triggered automation.

---

## Commands

| Command | Description |
|---------|-------------|
| `osc-record run` | Start the daemon |
| `osc-record setup` | Interactive setup wizard |
| `osc-record devices` | List available capture devices |
| `osc-record config` | Print resolved config |
| `osc-record manifest` | Generate a JSON manifest of recorded clips |
| `osc-record version` | Print version |

### `run` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `8000` | OSC UDP listen port |
| `--output` | `~/Dropbox/osc-record` | Output directory |
| `--prefix` | `recording` | Filename prefix |
| `--profile` | `h264` | Encoding profile: `h264`, `hevc`, `prores` |
| `--video-device` | from config | Override video device name |
| `--capture-mode` | `auto` | `auto`, `decklink`, `avfoundation`, `dshow` |
| `--no-tui` | `false` | Disable TUI, run as plain daemon |
| `--config` | see below | Path to config file |

Default config path: `~/.config/osc-record/config.toml` (macOS/Linux), `%APPDATA%\osc-record\config.toml` (Windows).

---

## Configuration

Config is auto-created on first run (or via `osc-record setup`).

### Single device

```toml
[osc]
port = 8000
record_address = "/start/record/"
stop_address = "/stop/record/"

[device]
capture_mode = "decklink"          # decklink | avfoundation | dshow | auto
name = "UltraStudio Recorder 3G"
format_code = "Hp59"               # decklink only — see Format Codes below
video_input = ""                   # decklink only: sdi | hdmi | component | auto

[recording]
profile = "h264"                   # h264 | hevc | prores
prefix = "SHOW"
output_dir = "~/Dropbox/recordings/"

[ffmpeg]
path = ""                          # leave empty to use ffmpeg from $PATH
```

### Multi-device (synchronized)

Use `[[devices]]` array instead of `[device]`. One OSC trigger starts and stops all devices simultaneously.

```toml
[osc]
port = 8000
record_address = "/start/record/"
stop_address = "/stop/record/"

[[devices]]
name = "UltraStudio Recorder 3G"
capture_mode = "decklink"
format_code = "24ps"
video_input = "hdmi"

[[devices]]
name = "UltraStudio 4K Mini"
capture_mode = "decklink"
format_code = "4k24"

[[devices]]
name = "HD Pro Webcam C920"
capture_mode = "avfoundation"
audio = "HD Pro Webcam C920"

[recording]
profile = "h264"
prefix = "SHOW"
output_dir = "~/Dropbox/recordings/"
```

Output files are named `{prefix}-{DeviceShortName}-{YYYY-MM-DD-HHmmss}.mp4`. With a single device, the short name is omitted.

---

## Format Codes (DeckLink)

Most DeckLink devices autodetect the incoming signal. Some (including UltraStudio Recorder 3G over SDI) require `format_code` to be set explicitly. If you get `Cannot Autodetect input stream`, set this.

List the codes supported by your device:

```sh
ffmpeg -f decklink -list_formats 1 -i "Your Device Name"
```

Common codes:

| Code | Format |
|------|--------|
| `Hp59` | 1080p 59.94 fps |
| `Hp50` | 1080p 50 fps |
| `Hp30` | 1080p 30 fps |
| `Hp29` | 1080p 29.97 fps |
| `Hp25` | 1080p 25 fps |
| `24ps` | 1080p 24 fps (rec709) |
| `23ps` | 1080p 23.976 fps |
| `Hi59` | 1080i 59.94 fps |
| `Hi50` | 1080i 50 fps |
| `4k24` | 2160p 24 fps |
| `4k25` | 2160p 25 fps |
| `4k30` | 2160p 30 fps |

---

## TUI Overview

The TUI has five panels:

```
┌─ SIGNAL ──────────────────────────┐  ┌─ STATUS ───────────────────────────┐
│ ● SDI  1920×1080  24.00 fps       │  │ IDLE                               │
│ L ███████░░░░░░░░  -42 dBFS       │  │ Device: UltraStudio Recorder 3G    │
│ R ███████░░░░░░░░  -44 dBFS       │  │ Profile: h264                      │
└───────────────────────────────────┘  └────────────────────────────────────┘
┌─ OSC ─────────────────────────────┐
│ Listening on :8000                │  ┌─ CLIPS ────────────────────────────┐
│ /start/record/ → 192.168.0.10     │  │ #1  SHOW-2026-03-14-200000.mp4 ✓  │
│ /stop/record/  → 192.168.0.10     │  │ #2  SHOW-2026-03-14-201500.mp4 ✓  │
└───────────────────────────────────┘  └────────────────────────────────────┘
┌─ LOG ─────────────────────────────────────────────────────────────────────┐
│ 20:00:00  Signal locked: 1080p24                                          │
│ 20:00:15  Recording started: SHOW-2026-03-14-200015.mp4                   │
└───────────────────────────────────────────────────────────────────────────┘
[R] Record  [S] Stop  [P] Preview  [V] View clip  [F1] Scan  [?] Help  [Q] Quit
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `R` | Start recording |
| `S` | Stop recording |
| `P` | Grab preview frame → open in system image viewer |
| `V` | Open last clip in system video player |
| `T` | Reset take counter |
| `F1` | Signal scanner — probe all format codes on the device |
| `F2` | Pre-show checklist overlay |
| `W` | Setup wizard overlay |
| `?` | Help overlay |
| `Q` | Quit (confirmation required if recording) |

---

## HTTP Status API

When `http.port` is set in config (e.g. `8080`), osc-record exposes:

| Endpoint | Description |
|----------|-------------|
| `GET /status` | Current recording state, active device, elapsed time |
| `GET /clips` | List of clips recorded this session |
| `GET /health` | `{"ok": true}` — for load balancer health checks |

```toml
[http]
port = 8080
```

---

## Capture Modes

**`decklink`** — Recommended for Blackmagic hardware. Uses ffmpeg's native DeckLink integration. Embeds SDI audio. Requires ffmpeg built with `--enable-decklink` and Blackmagic Desktop Video drivers installed. Auto-selects if supported.

**`avfoundation`** — macOS fallback. No special ffmpeg required. Set `audio` to the matching audio device name if needed.

**`dshow`** — Windows fallback. DirectShow equivalent.

**`auto`** — Tries decklink first. Falls back to avfoundation (macOS) or dshow (Windows) if decklink is unavailable.

---

## Requirements

- macOS 12+ or Windows 10+
- ffmpeg in `$PATH` (or set `ffmpeg.path` in config)
- **For DeckLink capture**: [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) 14.3+, ffmpeg with `--enable-decklink`

---

## License

MIT

---

## Troubleshooting

### Reinstall / upgrade

To fully reinstall (e.g. after a version bump):

```sh
brew uninstall osc-record ffmpeg-decklink
brew untap danielbrodie/tap
brew tap danielbrodie/tap
brew install osc-record
```

You must uninstall `ffmpeg-decklink` before `untap` — Homebrew refuses to remove a tap with installed formulae.

### ffmpeg not found

If you see `Error: ffmpeg not found on PATH`, osc-record looks for both `ffmpeg` and `ffmpeg-decklink` automatically. If neither is found:

```sh
brew tap danielbrodie/tap
brew install ffmpeg-decklink
```

Or point to any existing ffmpeg in your config:

```toml
[ffmpeg]
path = "/opt/homebrew/bin/ffmpeg"
```

### TUI doesn't launch / terminal too small

osc-record requires a minimum terminal width of 110 columns. If your terminal is smaller, use:

```sh
osc-record run --no-tui
```

### Blackmagic device not detected

- Ensure [Desktop Video](https://www.blackmagicdesign.com/support) drivers are installed
- Try unplugging and replugging the device
- Run `osc-record devices` to confirm detection
- If using SDI and autodetect fails, set `format_code` in config (see Format Codes above)
