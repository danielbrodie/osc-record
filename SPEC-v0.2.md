# osc-record v0.2 — Specification

## Overview

v0.2 adds a full terminal UI (TUI) as the primary interface for the `run` command. The TUI surfaces live capture state, OSC traffic, signal diagnostics, and recording status in a single screen. It replaces the ad-hoc `capture record` / `capture stop` flow with an integrated setup wizard, and adds a suite of production-grade features: pre-roll buffer, signal scanner, frame preview, pre-show checklist, multi-device support, HTTP status endpoint, and post-clip verification.

The TUI is the default when `run` is invoked from an interactive terminal. Non-interactive (piped, scripted) invocations retain the v0.1 plaintext behavior.

---

## Goals

- A TD or production manager can set up from zero to recording in under 2 minutes without reading docs
- Any failure mode — no device, no signal, wrong format, OSC not arriving, disk full — is visible on screen before the show starts
- A non-technical crew member can monitor the capture machine remotely or from across the room
- Editorial receives a per-show manifest with every clip, in/out, duration, and file size
- The pre-roll buffer saves clips even when the trigger fires late

---

## New Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/charmbracelet/bubbletea` | TUI event loop |
| `github.com/charmbracelet/lipgloss` | Styles, borders, colors |
| `github.com/charmbracelet/bubbles/viewport` | Scrollable log/OSC panels |
| `github.com/charmbracelet/bubbles/progress` | Disk usage bar |
| `github.com/charmbracelet/bubbles/spinner` | Scanning animation |
| `github.com/charmbracelet/bubbles/textinput` | Wizard / slate fields |
| `github.com/charmbracelet/bubbles/table` | Signal scanner results |
| `golang.org/x/term` | Terminal size detection |

Frame preview (optional, graceful degradation):
- Attempt sixel output (iTerm2, WezTerm, Kitty+sixel) via escape sequences
- Fall back to Unicode half-block art (▄▀ with ANSI color) for all other terminals
- If neither is supported or ffmpeg frame extraction fails, show "Preview unavailable"

---

## CLI Changes

### `run` command

`run` launches the TUI when stdout is a TTY. When stdout is not a TTY (piped or scripted), it falls back to v0.1 plaintext behavior.

New flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--no-tui` | false | Force plaintext mode even in a TTY |
| `--http-port` | 0 (disabled) | Enable HTTP status endpoint on this port |
| `--pre-roll` | 0 | Pre-roll buffer duration in seconds (0 = disabled) |
| `--devices` | (config) | Comma-separated list of device names for multi-device capture |
| `--confidence` | false | Open confidence monitor (ffplay fullscreen) on launch |

### Removed subcommands

`capture record` and `capture stop` are removed. Their function is replaced by the setup wizard in the TUI and `osc-record setup` (see below).

### New subcommand: `osc-record setup`

Non-interactive equivalent of the TUI wizard. Runs the pre-show checklist and setup flow in plaintext for scripted/CI environments. Saves config and exits.

### New subcommand: `osc-record manifest [dir]`

Scans a directory for osc-record clips (reads embedded metadata), generates a manifest `.txt` file, and prints it to stdout. Format: tab-separated columns of filename, show, scene, take, timecode-in, duration, file size, codec.

---

## TUI Layout

Terminal must be at least 100×30. If smaller, show a resize prompt.

### Main screen (normal state)

```
╭─ SIGNAL ──────────────────────────────╮  ╭─ OSC ──────────────────────────────────────╮
│ ● SDI   1080p59.94  Hp59   LOCKED     │  │ 11:04:12.441  /record/start   192.168.0.5  │
│ ○ HDMI  no signal                     │  │ 11:04:45.112  /record/stop    192.168.0.5  │
│                                       │  │ 11:05:01.008  /hello/felix/   192.168.0.5  │
│ L  ████████████░░░░░░  -12 dBFS       │  │                                            │
│ R  ███████████░░░░░░░  -14 dBFS       │  │                                            │
│                                       │  ╰────────────────────────────────────────────╯
│ TC  10:04:22:15                       │  ╭─ CLIPS ─────────────────────────────────────╮
╰───────────────────────────────────────╯  │ #1  Hamilton-Act2-3.mp4   04:22  247MB  ✓  │
╭─ STATUS ──────────────────────────────────│ #2  Hamilton-Act2-4.mp4   00:00  --    … │
│ ● RECORDING   Hamilton-Act2-3.mp4         ╰────────────────────────────────────────────╯
│ 00:04:22   247MB                     Device: UltraStudio Recorder 3G  Decklink  Hp59
│ ~/Dropbox/recordings/   847GB free   ~94h remaining at current rate
╰────────────────────────────────────────────────────────────────────────────────────────╯
╭─ LOG ──────────────────────────────────────────────────────────────────────────────────╮
│ 11:04:12  Recording started: Hamilton-Act2-3.mp4                                       │
│ 11:03:55  Pre-show checklist passed (6/6)                                              │
│ 11:03:50  OSC listener bound on :8000                                                  │
╰────────────────────────────────────────────────────────────────────────────────────────╯
  [R] Record  [S] Stop  [P] Preview  [F1] Signal scan  [F2] Checklist  [C] Confidence  [?]
```

### Color palette

| Element | Color |
|---------|-------|
| Background | `#0d0d0d` (near black) |
| Panel borders | `#2a2a2a` (dark grey) |
| Panel title | `#f5c400` (amber) |
| LOCKED / RECORDING / OK | `#00e676` (green) |
| WARNING / partial | `#ffab40` (amber-orange) |
| ERROR / no signal | `#ff5252` (red) |
| Idle state | `#78909c` (steel blue-grey) |
| VU meter fill | `#00e676` → `#ffab40` → `#ff5252` (green→amber→red at -6dB / 0dB) |
| OSC address | `#80d8ff` (light blue) |
| Timecode | `#e040fb` (purple) |
| Key hints | `#546e7a` (muted) |

All panels use rounded corners (`╭╮╰╯│─`). Recording state uses a blinking `●` (500ms interval).

---

## Panels

### Signal panel

Displays the current capture device status:

- **Input rows**: one row per physical input (SDI, HDMI). `●` = locked (green), `○` = no signal (red). When locked: resolution, framerate, format code.
- **VU meters**: two horizontal bar meters (L/R) updated at 100ms intervals from a running ffmpeg probe process. Meter segments: `█` (filled), `░` (empty). Color-coded: green below -6dBFS, amber -6 to 0, red at clip.
- **Timecode**: displays embedded LTC/VITC timecode from the SDI signal if present. Format `HH:MM:SS:FF`. Hidden if not detected.

Signal state is polled by a background goroutine that runs a 1-second ffmpeg probe every 5 seconds. Format: `ffmpeg -f decklink -format_code {code} -i {device} -t 1 -f null -` with stderr parsed for resolution/fps.

### OSC monitor panel

Scrollable viewport. Every incoming OSC packet appended in real time:

```
HH:MM:SS.mmm  /address/path  [args...]  source-IP
```

- Timestamps in local time, millisecond precision
- Arguments printed as space-separated values (int, float, string, blob)
- The configured record and stop addresses are highlighted in green/red respectively
- Unknown addresses shown in dim grey
- Source IP shown — helps diagnose "is this coming from QLab or something else"
- Scroll with arrow keys or mouse wheel; auto-scroll to bottom when not manually scrolled

### Recording status panel

Full-width bar below the signal/OSC split:

- **State indicator**: `● RECORDING` (blinking green) or `○ IDLE` (grey) or `◌ STARTING` (amber)
- **Current file**: filename, elapsed duration `HH:MM:SS`, file size (updated every 2s)
- **Device line**: device name, capture mode, format code
- **Disk line**: output directory, free space, estimated time remaining at current bitrate

### Clips panel

Session clip list (right side, below OSC):

- One row per completed or in-progress clip this session
- Columns: index, filename, duration, size, verification status (`✓` = ffprobe passed, `✗` = failed, `…` = in progress)
- Click or press Enter on a row to reveal in Finder (macOS) / Explorer (Windows)

### Log panel

Scrollable event log at the bottom. Entries:

- Recording started/stopped with filename
- Signal locked/lost events
- OSC trigger received (address + source)
- ffmpeg errors (parsed from stderr, deduped)
- Pre-roll buffer events
- Clip verification results
- Disk space warnings (at 10%, 5%, 1% remaining)
- Recovery events (ffmpeg restart, partial clip saved)

---

## Setup Wizard

Triggered automatically when config is missing, incomplete, or when the user presses `W` from the main screen.

### Step 1: Device selection

Show a table of all detected capture devices with real-time signal status:

```
  #   Device                     Mode       Signal
  1   UltraStudio Recorder 3G    decklink   ● SDI 1080p59.94
  2   HD Pro Webcam C920         avfound.   ○ no signal
```

Arrow keys to select, Enter to confirm. Auto-selects if exactly one device found.

### Step 2: Signal configuration (decklink only)

If the selected device is decklink and format_code is unset:
- First try autodetect (2-second probe)
- If autodetect fails: offer to run Signal Scanner (see below) to find the right format_code
- If autodetect succeeds: confirm detected format and continue

### Step 3: OSC addresses

```
Waiting for OSC...  listening on :8000

  Send your RECORD cue now:
  > (waiting)

  Most recent: /hello/felix/  from 192.168.0.5   [Enter to select]
```

Live OSC monitor embedded. Press Enter to select the most recent address as the record trigger. Repeat for stop trigger.

### Step 4: Output configuration

```
  Output directory:  ~/Dropbox/recordings/   [Tab to browse]
  Filename prefix:   recording
  Profile:           h264   [Tab to cycle: h264 / hevc / prores]
```

### Step 5: Pre-show checklist

Runs automatically after setup (see Checklist section). Must be all green before proceeding to main screen. User can skip with `S` (with confirmation).

---

## Signal Scanner

Accessible from setup wizard (step 2) or by pressing `F1` from the main screen.

Systematically tests every supported format code on every physical input. For each combination that locks, optionally captures a preview frame.

### Scan process

1. Build a test matrix: for each `video_input` in `[sdi, hdmi]` × for each format code in the device's supported list
2. For each combination, run: `ffmpeg -f decklink -video_input {input} -format_code {code} -i {device} -t 1 -f null -`
3. Timeout per test: 3 seconds
4. Parse result: locked / no signal / error
5. For locked results: run a frame capture (see Frame Preview) and show thumbnail in results table

### Results display

```
╭─ SIGNAL SCANNER ─────────────────────────────────────────────────────╮
│  Input  Format  Description              Status       Preview         │
│  ─────  ──────  ───────────────────────  ───────────  ─────────────── │
│  SDI    Hp59    1080p 59.94 fps          ✓ LOCKED     [▄▄▀▀▄▄▀▀▄▄]   │
│  SDI    Hp29    1080p 29.97 fps          ○ no signal                  │
│  SDI    Hi59    1080i 59.94 fps          ○ no signal                  │
│  HDMI   Hp59    1080p 59.94 fps          ○ no signal                  │
│  ...                                                                  │
│                                                                       │
│  Found 1 locked signal. Press Enter on a row to use it.              │
╰───────────────────────────────────────────────────────────────────────╯
  Scanning: SDI Hp50 ████████░░░░░░░░░  12/48 (3s remaining)
```

Arrow keys to select a result; Enter saves `video_input` and `format_code` to config and returns to main screen or wizard.

Scan can be interrupted with `Esc`.

---

## Frame Preview

Captures a single frame from the live capture device and displays it in the terminal.

### Implementation

```
ffmpeg -f decklink [-video_input {input}] [-format_code {code}] \
  -i {device} -vframes 1 -f image2pipe -vcodec mjpeg -
```

Output is a JPEG in memory. Rendering:

1. Detect terminal capabilities:
   - Check `$TERM`, `$TERM_PROGRAM`, `$COLORTERM`, `XTERM_VERSION`
   - iTerm2 / WezTerm / Kitty with sixel: use sixel output via escape sequences
   - Kitty: use Kitty graphics protocol
   - Anything else: convert to Unicode half-block art (▄▀ per 2 vertical pixels, ANSI 256-color)
2. Scale to fit available panel width, maintain aspect ratio
3. Display in a floating overlay panel with `[P]review` label and `[Esc] to close`

Accessible from:
- Signal scanner results (auto-shown for locked signals)
- Main screen via `P` key (previews current live signal)
- Clips panel via `V` key (previews first frame of a completed clip)

---

## Pre-show Checklist

Run automatically before the main screen appears, and accessible via `F2` at any time.

```
╭─ PRE-SHOW CHECKLIST ──────────────────────────────────────────────────╮
│                                                                        │
│  ✓  Device found          UltraStudio Recorder 3G                     │
│  ✓  Driver active         BlackmagicIO.DExt [activated enabled]       │
│  ✓  Signal locked         SDI 1080p59.94 Hp59                        │
│  ✓  OSC configured        /record/start  /record/stop                 │
│  ✓  Output dir writable   ~/Dropbox/recordings/                       │
│  ✓  Disk space            847GB free (~94h at h264 1080p)             │
│  ✗  ffmpeg decklink       not compiled with --enable-decklink         │
│                            → Install: brew install homebrew-ffmpeg/…  │
│                                                                        │
│  6/7 checks passed                     [S] Skip   [R] Retry   [Enter] │
╰────────────────────────────────────────────────────────────────────────╯
```

### Checks

| Check | Pass condition | Failure action |
|-------|---------------|----------------|
| Device found | At least one capture device detected | Show device picker |
| Driver active (macOS) | `BlackmagicIO.DExt` in `[activated enabled]` state | Show instructions to open Desktop Video Setup |
| Signal locked | Probe passes with configured format_code | Offer signal scanner |
| OSC configured | Both `record_address` and `stop_address` are set | Trigger setup wizard step 3 |
| Output dir writable | `os.MkdirAll` + test write succeeds | Show dir picker |
| Disk space | >5GB free | Warning at 10GB, error below 5GB |
| ffmpeg decklink | `ffmpeg -sources decklink` exits without "Unknown input format" | Show install instructions |

Driver active check is macOS-only; runs `systemextensionsctl list` and parses output.

---

## Pre-roll Buffer

When `--pre-roll N` (or `recording.pre_roll` in config) is set, osc-record continuously captures to a ring buffer of N seconds. On record trigger, the ring buffer is flushed to the output file before live capture continues, giving N seconds of "before the cue" footage.

### Implementation

- Background ffmpeg process writes to a segmented HLS stream in a temp directory: `-f hls -hls_time 1 -hls_list_size {N} -hls_flags delete_segments`
- On record trigger: concatenate the ring buffer segments + start normal output capture
- Total output file: ring buffer + live capture from trigger point onward
- On stop: finalize file, clean temp directory
- If ring buffer goroutine crashes: log event, fall back to normal capture, alert in TUI

Config:
```toml
[recording]
  pre_roll = 5  # seconds, 0 to disable
```

Maximum pre-roll: 30 seconds. Values above 30 are clamped with a warning.

---

## Clip Naming: Show / Scene / Take

Replace simple `prefix-timestamp` filename scheme with structured slate fields.

Config:
```toml
[recording]
  show  = ""   # e.g. "Hamilton"
  scene = ""   # e.g. "Act2"
  take  = ""   # auto-increments if empty; set manually to override
  prefix = ""  # legacy; used if show/scene/take all empty
```

Filename format: `{show}-{scene}-{take}-{YYYY-MM-DD-HHmmss}.{ext}`
If any field is empty: `{prefix}-{YYYY-MM-DD-HHmmss}.{ext}` (v0.1 behavior).

Take auto-increments per session (resets to 1 on new day or when manually reset with `T` in TUI).

In TUI, `N` opens a quick-edit overlay:
```
╭─ CLIP NAME ─────────────╮
│  Show:   Hamilton        │
│  Scene:  Act2            │
│  Take:   4 (auto)        │
╰─────────────────────────╯
```

Changes take effect on the next record trigger.

Filename is embedded in the output file as metadata:
```
ffmpeg ... -metadata show="{show}" -metadata scene="{scene}" -metadata take="{take}"
```

---

## Multi-device Capture

When `--devices "Device A,Device B"` is passed (or `device.names = [...]` in config), osc-record starts a capture goroutine per device. All goroutines share the same OSC listener; a single record trigger fires all simultaneously.

### TUI changes

Signal panel expands to show one row per device. VU meters show for each. Recording status shows one row per device.

Clips panel shows clips grouped by device.

### Config

```toml
[[devices]]
  name = "UltraStudio Recorder 3G"
  format_code = "Hp59"
  audio = ""

[[devices]]
  name = "HD Pro Webcam C920"
  format_code = ""
  audio = "HD Pro Webcam C920"
```

Single-device config (v0.1 `[device]` block) remains valid and is converted internally.

---

## Confidence Monitor

Press `C` from the main screen (or pass `--confidence` flag) to launch a fullscreen ffplay window showing the live capture signal.

```
ffplay -f decklink [-format_code {code}] -i {device} -fs
```

Window title: "osc-record confidence — {device}".

Closes when `Q` or `Esc` is pressed in the ffplay window, or when osc-record exits.

macOS only in v0.2 (ffplay fullscreen on Windows has known issues). On Windows: show a warning and skip.

---

## HTTP Status Endpoint

When `--http-port N` is set (or `http.port = N` in config), osc-record exposes a minimal HTTP server.

### `GET /status`

```json
{
  "state": "recording",
  "file": "Hamilton-Act2-3.mp4",
  "duration_s": 262,
  "size_bytes": 247000000,
  "disk_free_bytes": 909868457984,
  "device": "UltraStudio Recorder 3G",
  "signal_locked": true,
  "format": "Hp59",
  "clips_this_session": 2,
  "pre_roll_s": 5,
  "osc_port": 8000,
  "record_address": "/record/start",
  "stop_address": "/record/stop"
}
```

### `GET /clips`

Array of clip objects for the current session.

### `GET /health`

Returns `200 OK` with body `ok` when the daemon is running and no critical errors are active. Returns `503` if signal is lost or ffmpeg has crashed.

No authentication. Intended for local network monitoring (stage manager's iPad, status display). Do not expose to public internet.

Config:
```toml
[http]
  port = 0      # 0 = disabled
  bind = "0.0.0.0"  # restrict to loopback with "127.0.0.1"
```

---

## Clip Verification

After every stop, run ffprobe on the completed file:

```
ffprobe -v error -show_streams -show_format -of json {file}
```

Checks:
- File is readable
- At least one video stream present
- At least one audio stream present (unless capture mode is known to be video-only)
- Duration matches elapsed recording time (within 2-second tolerance)
- No `codec_tag_string = "NONE"` (indicates corrupt encoding)

If all checks pass: mark clip `✓` in the clips panel, log "Clip verified: {file}".
If any check fails: mark clip `✗`, log the specific failure, show an alert in the TUI (amber banner), and write the partial/corrupt clip path to a `{session}-failed-clips.txt` file in the output directory.

---

## Post-show Manifest

On exit (or via `osc-record manifest [dir]`), write a session manifest to the output directory:

Filename: `{show}-{YYYY-MM-DD}-manifest.txt` (or `session-manifest.txt` if show is unset).

Format:
```
osc-record session manifest
Generated: 2026-03-13 19:04:22
Show:      Hamilton
Device:    UltraStudio Recorder 3G  (decklink  Hp59)
Output:    ~/Dropbox/recordings/

#   File                          Scene  Take  TC In         Duration  Size    Status
1   Hamilton-Act2-3.mp4           Act2   3     10:04:22:15   04:22     247MB   ✓
2   Hamilton-Act2-4.mp4           Act2   4     10:10:01:00   02:11     124MB   ✓
3   Hamilton-Act2-5-partial.mp4   Act2   5     10:14:33:08   00:44     42MB    ✗ (ffprobe: no audio)

Total: 3 clips, 9:17, 413MB
```

Manifest is also printed to stdout on exit if `--manifest` flag is passed.

---

## Recovery: ffmpeg Crash Handling

If the recording ffmpeg process exits unexpectedly:

1. Log event: `[HH:MM:SS] ffmpeg exited unexpectedly (code N). Partial clip: {file}`
2. Run ffprobe on partial file to determine if it's recoverable
3. Mark clip `⚠ partial` in clips panel
4. Attempt to restart capture (probe + re-open device)
5. If restart succeeds within 10 seconds: log "Capture resumed", wait for next record trigger
6. If restart fails: show error banner in TUI, log failure reason, do not exit daemon

Signal loss mid-recording (device disconnects):
- ffmpeg will exit with I/O error
- Same recovery flow as above
- Log: `[HH:MM:SS] Signal lost mid-recording. Partial clip saved: {file}`

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `R` | Start recording (manual override — bypasses OSC) |
| `S` | Stop recording (manual override) |
| `P` | Frame preview of current signal |
| `V` | Frame preview of selected clip (in clips panel) |
| `N` | Edit clip name (show/scene/take) |
| `T` | Reset take counter to 1 |
| `C` | Open confidence monitor |
| `W` | Open setup wizard |
| `F1` | Signal scanner |
| `F2` | Pre-show checklist |
| `Tab` | Cycle focus between panels |
| `↑↓` | Scroll focused panel |
| `Enter` | Select / confirm |
| `Esc` | Close overlay / cancel |
| `?` | Help overlay |
| `Q` / `Ctrl+C` | Quit (with confirmation if recording) |

---

## Config Changes

Full v0.2 config schema (backwards compatible with v0.1):

```toml
[osc]
  port = 8000
  record_address = ""
  stop_address = ""

[device]
  capture_mode = "auto"
  name = ""
  audio = ""
  format_code = ""

# Multi-device (optional — replaces [device] if present)
# [[devices]]
#   name = "UltraStudio Recorder 3G"
#   format_code = "Hp59"
#
# [[devices]]
#   name = "HD Pro Webcam C920"
#   audio = "HD Pro Webcam C920"

[recording]
  profile = "h264"
  prefix = "recording"
  output_dir = "~/Dropbox/osc-record/"
  show  = ""
  scene = ""
  take  = ""      # empty = auto-increment
  pre_roll = 0    # seconds

[http]
  port = 0
  bind = "0.0.0.0"

[ffmpeg]
  path = ""

[tui]
  enabled = true  # false to always use plaintext
  theme = "dark"  # reserved for future light theme
```

---

## Project Structure Changes

```
osc-record/
├── cmd/
│   ├── root.go
│   ├── run.go           # detect TTY, launch TUI or plaintext
│   ├── setup.go         # new: osc-record setup subcommand
│   ├── manifest.go      # new: osc-record manifest subcommand
│   ├── devices.go
│   ├── version.go
│   └── config_cmd.go
├── internal/
│   ├── capture/         # unchanged from v0.1
│   ├── config/
│   │   └── config.go    # extended schema
│   ├── devices/         # unchanged from v0.1
│   ├── osc/             # unchanged from v0.1
│   ├── recorder/        # extended: pre-roll, multi-device, verification
│   ├── tui/             # new package
│   │   ├── model.go     # bubbletea root model
│   │   ├── signal.go    # signal panel component
│   │   ├── oscmon.go    # OSC monitor panel
│   │   ├── status.go    # recording status panel
│   │   ├── clips.go     # clips panel
│   │   ├── log.go       # log panel
│   │   ├── scanner.go   # signal scanner overlay
│   │   ├── preview.go   # frame preview overlay
│   │   ├── checklist.go # pre-show checklist overlay
│   │   ├── wizard.go    # setup wizard
│   │   ├── slate.go     # clip naming overlay
│   │   ├── styles.go    # lipgloss styles, palette
│   │   └── keys.go      # keyboard bindings
│   ├── preview/         # new: frame extraction + sixel/unicode rendering
│   │   ├── capture.go
│   │   └── render.go
│   ├── manifest/        # new: manifest generation
│   │   └── manifest.go
│   ├── health/          # new: HTTP status server
│   │   └── server.go
│   └── platform/        # extended: driver check (macOS systemextensionsctl)
│       ├── platform.go
│       ├── darwin.go
│       └── windows.go
├── Formula/
│   └── osc-record.rb
├── .goreleaser.yml
├── SPEC.md
├── SPEC-v0.2.md
├── README.md
├── go.mod
└── main.go
```

---

## Out of Scope for v0.2

- Light theme
- Timecode chase / jam sync
- NDI / network input sources
- Cloud upload of completed clips
- iOS remote control app
- Windows confidence monitor (ffplay fullscreen issues)
- RTMP/HLS streaming output
- Multi-track audio (>2ch)

---

## Version

`v0.2.0`. Build tag: `go:build !v01_compat` (no backward-incompatible breaks; v0.1 configs load cleanly).
