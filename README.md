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

### macOS

```sh
brew tap danielbrodie/tap
brew install osc-record
```

This installs `osc-record` and a pre-built ffmpeg with DeckLink support. You also need [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) drivers (14.3+) installed.

**Upgrade:**

```sh
brew update && brew upgrade osc-record
```

**Reinstall** if something is broken:

```sh
brew reinstall osc-record ffmpeg-decklink
```

<details>
<summary>Full clean install (if reinstall doesn't fix it)</summary>

```sh
brew uninstall osc-record ffmpeg-decklink
brew untap danielbrodie/tap
brew tap danielbrodie/tap
brew install osc-record
```
</details>

### Windows

**Via Scoop (recommended):**

```powershell
scoop bucket add danielbrodie https://github.com/danielbrodie/homebrew-tap
scoop install osc-record
```

This installs `osc-record` and ffmpeg automatically. You also need [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) drivers (14.3+) for DeckLink capture.

**Upgrade:**

```powershell
scoop update osc-record
```

<details>
<summary>Manual install (without Scoop)</summary>

1. Install ffmpeg from [gyan.dev](https://www.gyan.dev/ffmpeg/builds/) and add it to your PATH.
2. Download `osc-record_windows_amd64.zip` from [Releases](https://github.com/danielbrodie/osc-record/releases).
3. Extract and place `osc-record.exe` somewhere on your PATH (e.g. `C:\Users\You\bin\`).
4. Install [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) drivers if using DeckLink hardware.
</details>

---

## Quick Start

### 1. Run setup

```sh
osc-record setup
```

The setup wizard walks you through: picking your capture device, choosing your video input (HDMI, SDI, or auto-detect), configuring your record/stop OSC addresses (it listens for a real OSC message so you just fire the cue), setting output directory and filename prefix. Config is saved automatically.

For headless/automation environments:

```sh
osc-record setup --no-tui
```

### 2. Start recording

```sh
osc-record run
```

On first run with a DeckLink device, osc-record **auto-detects** the video input (HDMI/SDI) and format code — no manual configuration needed. The signal panel shows `⟳ probing...` while detection runs, then transitions to a locked signal. Detected values are saved to config so subsequent runs skip detection.

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
format_code = ""                   # decklink only — auto-detected on first run
video_input = ""                   # decklink only — auto-detected on first run (sdi | hdmi)

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

As of v1.2.0, osc-record **auto-detects** the format code on first run. The detected value is saved to `config.toml` — clear `format_code` to re-trigger detection.

If auto-detection fails or you want to set it manually, list the codes supported by your device:

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
| `N` | Slate — set show, scene, take metadata |
| `T` | Reset take counter |
| `P` | Grab preview frame → open in system image viewer |
| `V` | Open last clip in system video player |
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

### Disguise / d3 OSC

Disguise sends OSC inside **bundle packets** (`#bundle`). osc-record handles bundles correctly as of v1.1.5. If setup/recording never triggers from Disguise, upgrade: `brew upgrade osc-record`

### ffmpeg not found

osc-record looks for both `ffmpeg` and `ffmpeg-decklink` on your PATH. If neither is found, reinstall (see [Install](#install) above) or point to an existing ffmpeg:

```toml
[ffmpeg]
path = "/opt/homebrew/bin/ffmpeg"      # macOS
# path = "C:/ffmpeg/bin/ffmpeg.exe"   # Windows
```

### Blackmagic device not found on Windows (`osc-record devices` shows nothing)

Ensure [Desktop Video](https://www.blackmagicdesign.com/support) drivers are installed. Then run:

```powershell
osc-record devices
```

You should see `Blackmagic WDM Capture` under Video devices. If not, unplug and replug the device. Note: on Windows, osc-record uses dshow mode (via the Blackmagic WDM driver) rather than native decklink mode — signal format is auto-detected from the incoming signal.

### TUI doesn't launch / terminal too small

osc-record requires a minimum terminal width of 110 columns. If your terminal is smaller, use:

```sh
osc-record run --no-tui
```

### Blackmagic device not detected

- Ensure [Desktop Video](https://www.blackmagicdesign.com/support) drivers are installed
- Try unplugging and replugging the device
- Run `osc-record devices` to confirm detection

### Auto-detect didn't find a signal

- Ensure a camera is connected and powered on before running `osc-record run`
- Clear `video_input` and `format_code` in config to re-trigger detection
- Use `F1` in the TUI to manually scan format codes
- If auto-detect consistently fails, set `format_code` and `video_input` manually (see Format Codes above)
