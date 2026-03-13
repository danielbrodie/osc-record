# osc-record

A cross-platform CLI tool that listens for OSC messages and triggers ffmpeg-based video recording from a capture device. Designed for live theatrical production environments where a lighting console or media server (Disguise, QLab, etc.) sends OSC cues to start and stop recording a camera feed.

## Install

Distributed via Homebrew tap and as prebuilt binaries for macOS (arm64, amd64) and Windows (amd64).

```
brew tap brodiegraphics/tools
brew install osc-record
```

Or download a release binary from GitHub.

### Runtime Dependencies

- **ffmpeg** must be installed and available on PATH (or path set in config)
- **Blackmagic Desktop Video drivers** must be installed if using a Blackmagic capture device
- **For decklink capture (recommended):** ffmpeg must be compiled with `--enable-decklink`, which requires the Blackmagic DeckLink SDK headers. The Homebrew formula handles this automatically. If using a manual ffmpeg install, see the Capture Modes section below.

---

## CLI Interface

Binary name: `osc-record`

### Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `~/.config/osc-record/config.toml` | Path to config file |
| `--verbose` | bool | false | Verbose logging to stderr |

### Commands

#### `osc-record devices`

Lists available capture devices from ffmpeg. Probes multiple input formats in order of preference.

On macOS, probes:
1. `ffmpeg -f decklink -list_devices true -i ""` (preferred, if decklink support is compiled in)
2. `ffmpeg -f avfoundation -list_devices true -i ""` (fallback)

On Windows, probes:
1. `ffmpeg -f decklink -list_devices true -i ""` (preferred, if decklink support is compiled in)
2. `ffmpeg -f dshow -list_devices true -i dummy` (fallback)

If ffmpeg does not support the decklink input format (exits with "Unknown input format"), skip it silently and fall back. Parses ffmpeg stderr output and prints a clean list grouped by capture mode. Example output:

```
Capture mode: decklink (auto-detect signal format)

DeckLink devices:
  UltraStudio Recorder

Capture mode: avfoundation (manual format required)

Video devices:
  [0] Blackmagic UltraStudio Recorder
  [1] FaceTime HD Camera

Audio devices:
  [0] Blackmagic Audio
  [1] MacBook Pro Microphone
```

If only one capture mode is available, only that section is shown.

#### `osc-record capture record`

Enters a temporary OSC listener mode. Binds to the configured port (default 8000). Prints every incoming OSC message address and arguments to stdout in real time. Example output:

```
Listening for OSC on port 8000... Press Enter to select, Ctrl+C to cancel.

  /cue/1/start []
  /cue/1/start []        <-- most recent
```

When the user presses Enter, the most recently received OSC address is saved to `config.toml` as `osc.record_address`. If no message has been received, print an error and exit.

On success, print:
```
Saved record trigger: /cue/1/start
```

#### `osc-record capture stop`

Identical behavior to `capture record`, but saves to `osc.stop_address`.

On success, print:
```
Saved stop trigger: /cue/2/start
```

#### `osc-record run`

The main daemon mode. Reads config, binds OSC listener, waits for triggers, records via ffmpeg.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--prefix` | string | `"recording"` | Filename prefix prepended to date |
| `--profile` | string | `"h264"` | Recording profile: `prores`, `hevc`, or `h264` |
| `--output` | string | `~/Dropbox/osc-record/` | Output directory for recordings |
| `--port` | int | from config or 8000 | Override OSC listen port |
| `--capture-mode` | string | `"auto"` | Capture mode: `auto`, `decklink`, `avfoundation`, or `dshow` |
| `--video-device` | string | from config | Override video device (index or name) |
| `--audio-device` | string | from config | Override audio device (index or name, ignored in decklink mode) |

Behavior:

1. Validate that ffmpeg is available. Exit with error if not found.
2. Validate that `osc.record_address` and `osc.stop_address` are configured. Exit with error if not.
3. Resolve capture mode (see Capture Modes section below). Mode is selected automatically — decklink wins if ffmpeg supports it; user is never asked to pick a mode.
4. **Interactive device picker (if `device.name` is unset and no `--video-device` flag):**
   - Probe available devices for the resolved capture mode.
   - Display a numbered list. For decklink: video devices only (audio is embedded in SDI). For avfoundation/dshow: video devices first, then a second prompt for audio device.
   - Example (decklink):
     ```
     No capture device configured. Available devices:

       [1] UltraStudio Recorder
       [2] DeckLink Mini Recorder 4K

     Select device [1-2]:
     ```
   - Example (avfoundation):
     ```
     No capture device configured. Available video devices:

       [1] Blackmagic UltraStudio Recorder
       [2] FaceTime HD Camera

     Select video device [1-2]: 1

     Available audio devices:

       [1] Blackmagic Audio
       [2] MacBook Pro Microphone

     Select audio device [1-2]:
     ```
   - Save selection to `device.name` (and `device.audio` if avfoundation/dshow) in config.toml.
   - If no devices are found, exit with error: `Error: No capture devices found. Run 'osc-record devices' for details.`
   - If exactly one device is found, select it automatically without prompting and print: `Auto-selected device: UltraStudio Recorder`
5. Run 2-second signal probe (decklink mode only — see Capture Modes). Probe runs before OSC port is bound.
6. Create output directory if it doesn't exist.
7. Bind OSC listener on configured port.
8. Print startup summary to stdout:
   ```
   osc-record running
     OSC port:    8000
     Record:      /cue/1/start
     Stop:        /cue/2/start
     Capture:     decklink (auto-detect)
     Profile:     h264
     Prefix:      CATS
     Output:      /Users/daniel/Dropbox/osc-record/
     Device:      UltraStudio Recorder
   
   Waiting for record trigger...
   ```
9. On receiving the record OSC address: spawn ffmpeg process (details below), print `Recording started: CATS-2026-03-13-193022.mp4`
10. On receiving the stop OSC address: send SIGINT (macOS/Linux) or write `q` to ffmpeg stdin (Windows) to cleanly stop recording. Wait for ffmpeg to exit. Print `Recording saved: CATS-2026-03-13-193022.mp4`
11. Return to step 9, waiting for next record trigger.

Edge cases:
- If a record trigger arrives while already recording: ignore it, log a warning.
- If a stop trigger arrives while not recording: ignore it, log a warning.
- If ffmpeg exits unexpectedly during recording: log an error, return to waiting state.
- SIGTERM/SIGINT to the osc-record process itself: if recording, stop ffmpeg cleanly first, then exit.

#### `osc-record config`

Prints the current resolved configuration (merged from file + defaults) as TOML to stdout.

#### `osc-record version`

Prints version string. Format: `osc-record v0.1.0`

---

## Configuration

File: `~/.config/osc-record/config.toml`

Auto-created with defaults on first run of any command if it doesn't exist.

```toml
[osc]
port = 8000
record_address = ""    # set by 'capture record'
stop_address = ""      # set by 'capture stop'

[device]
capture_mode = "auto"  # "auto", "decklink", "avfoundation", "dshow"
name = ""              # device name (decklink) or video device index/name (avfoundation/dshow)
audio = ""             # audio device index or name (avfoundation/dshow only, ignored for decklink)

[recording]
profile = "h264"       # "prores", "hevc", "h264"
prefix = "recording"
output_dir = "~/Dropbox/osc-record/"

[ffmpeg]
path = ""              # empty = find on PATH
```

CLI flags override config file values. Config file values override defaults.

---

## Capture Modes

osc-record supports two capture paths. The `auto` mode (default) tries decklink first and falls back to the platform's native framework.

### Decklink Mode (Preferred)

Uses ffmpeg's `-f decklink` input format, which is Blackmagic's own ffmpeg integration. The key advantage: **decklink auto-detects the incoming signal format.** No need to specify framerate, resolution, or pixel format. ffmpeg queries the device, reads whatever the camera is sending (1080i59.94, 1080p29.97, 4K30, etc.), and captures it directly.

Audio is embedded in the decklink stream (SDI carries audio natively), so no separate audio device is needed.

Requirement: ffmpeg must be compiled with `--enable-decklink` and the DeckLink SDK headers. The Homebrew formula uses `homebrew-ffmpeg/ffmpeg` tap which supports this option.

**Auto-detect resolution:** At startup in `run` mode, osc-record runs a 2-second probe capture to confirm the device is receiving a valid signal. This runs before the OSC port is bound (so no triggers can arrive during the probe). This catches "camera not connected" before the show starts rather than on the first cue.

```
ffmpeg -f decklink -i "UltraStudio Recorder" -t 2 -f null -
```

If this fails, print:
```
Warning: No valid signal detected on "UltraStudio Recorder". Recording will fail until a signal is present.
```

### AVFoundation Mode (macOS Fallback)

Used when decklink is not available (ffmpeg not compiled with decklink support, or user explicitly sets `capture_mode = "avfoundation"`). Requires manual specification of framerate and pixel format, and a separate audio device reference.

### DirectShow Mode (Windows Fallback)

Windows equivalent of AVFoundation. Same limitations: manual format specification required, separate audio device reference.

### Auto Mode Resolution

When `capture_mode = "auto"` (the default), the mode is selected automatically — the user is never asked to choose:

1. Test if ffmpeg supports decklink: run `ffmpeg -f decklink -list_devices true -i ""` and check exit behavior.
2. If supported, use decklink mode (regardless of whether a device is configured yet — device selection happens separately via the interactive picker).
3. Otherwise, fall back to avfoundation (macOS) or dshow (Windows).
4. Print the resolved mode in the startup summary so the operator knows what's happening.

---

## Recording Profiles

Each profile has a decklink variant and a fallback variant. The only difference is the input arguments; the output codec is the same.

### ProRes LT (`prores`)

Container: `.mov`

Decklink:
```
ffmpeg -f decklink -i "{device_name}" \
  -c:v prores_ks -profile:v 1 -c:a pcm_s16le \
  "{output_path}"
```

AVFoundation fallback:
```
ffmpeg -f avfoundation -framerate 29.97 -pixel_format uyvy422 \
  -i "{video_device}:{audio_device}" \
  -c:v prores_ks -profile:v 1 -c:a pcm_s16le \
  "{output_path}"
```

DirectShow fallback:
```
ffmpeg -f dshow \
  -i video="{video_device}":audio="{audio_device}" \
  -c:v prores_ks -profile:v 1 -c:a pcm_s16le \
  "{output_path}"
```

Notes:
- `-profile:v 1` is ProRes LT. Reasonable balance of quality and file size for archival.
- Audio is uncompressed PCM. Standard for ProRes workflows.
- In decklink mode, framerate and pixel format are auto-detected from the signal. No manual specification needed.
- In fallback modes, `-framerate 29.97` and `-pixel_format uyvy422` are hardcoded defaults. If the signal doesn't match, ffmpeg will fail. This is the primary reason decklink mode is preferred.
- On Windows, `prores_ks` encoder works but playback requires QuickTime or VLC. Warn the user on profile selection if on Windows.

### H.264 (`h264`)

Container: `.mp4`

Decklink:
```
ffmpeg -f decklink -i "{device_name}" \
  -c:v libx264 -crf 18 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

AVFoundation fallback:
```
ffmpeg -f avfoundation -framerate 29.97 -pixel_format uyvy422 \
  -i "{video_device}:{audio_device}" \
  -c:v libx264 -crf 18 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

DirectShow fallback:
```
ffmpeg -f dshow \
  -i video="{video_device}":audio="{audio_device}" \
  -c:v libx264 -crf 18 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

Notes:
- CRF 18 is visually lossless for most content. Small enough for Dropbox sync over venue wifi.
- `-preset fast` balances CPU load vs. compression. Live capture on a Mac Mini should handle this fine.
- AAC audio at 192k.

### HEVC (`hevc`)

Container: `.mp4`

Decklink:
```
ffmpeg -f decklink -i "{device_name}" \
  -c:v libx265 -crf 22 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

AVFoundation fallback:
```
ffmpeg -f avfoundation -framerate 29.97 -pixel_format uyvy422 \
  -i "{video_device}:{audio_device}" \
  -c:v libx265 -crf 22 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

DirectShow fallback:
```
ffmpeg -f dshow \
  -i video="{video_device}":audio="{audio_device}" \
  -c:v libx265 -crf 22 -preset fast -c:a aac -b:a 192k \
  "{output_path}"
```

Notes:
- CRF 22 for HEVC is roughly equivalent quality to CRF 18 for H.264 at ~40% smaller file size.
- Higher CPU load than H.264. Should still be fine on Apple Silicon Mac Mini.
- Some older playback systems may not support HEVC. H.264 is the safer default.

---

## Platform-Specific Behavior

### macOS

- Primary capture: `decklink` (auto-detect signal format, embedded audio)
- Fallback capture: `avfoundation` (manual framerate/pixel_format, separate audio device)
- Device discovery: decklink first, then avfoundation (see `devices` command)
- Stopping ffmpeg: send `SIGINT` to the process
- Config directory: `~/.config/osc-record/`
- Tilde expansion (`~`) for output_dir resolves via `os.UserHomeDir()`

### Windows

- Primary capture: `decklink` (auto-detect signal format, embedded audio)
- Fallback capture: `dshow` (manual format, separate audio device by name only)
- Device discovery: decklink first, then dshow (see `devices` command)
- Stopping ffmpeg: write `q` to ffmpeg's stdin pipe (SIGINT is unreliable on Windows)
- Config directory: `%APPDATA%\osc-record\config.toml`
- ProRes note: ffmpeg's `prores_ks` encoder works on Windows but playback requires QuickTime or VLC. Warn the user on profile selection if on Windows.

### Decklink Mode (Both Platforms)

When using decklink mode, the platform differences collapse to just how you stop ffmpeg (SIGINT vs. stdin `q`). Everything else is identical: same input format string, same device naming, same auto-detection behavior. This is a significant simplification.

---

## File Naming

Pattern: `{prefix}-{YYYY-MM-DD-HHmmss}.{ext}`

- `prefix` comes from `--prefix` flag or config
- Timestamp is local time at recording start
- Extension is determined by profile (`.mov` for prores, `.mp4` for h264/hevc)
- Characters in prefix that are filesystem-unsafe (`/ \ : * ? " < > |`) are stripped

Example: `CATS-GuestJudges-2026-03-13-193022.mp4`

---

## Project Structure

```
osc-record/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go           # root command, global flags, config loading
│   ├── devices.go        # 'devices' subcommand
│   ├── capture.go        # 'capture record' and 'capture stop'
│   ├── run.go            # 'run' subcommand (main daemon)
│   ├── config_cmd.go     # 'config' subcommand (print config)
│   └── version.go        # 'version' subcommand
├── internal/
│   ├── config/
│   │   └── config.go     # TOML config parsing, defaults, merging
│   ├── osc/
│   │   └── listener.go   # OSC server, message dispatch
│   ├── recorder/
│   │   └── recorder.go   # ffmpeg process management, start/stop/cleanup
│   ├── devices/
│   │   └── devices.go    # ffmpeg device listing, parsing (decklink + fallback)
│   ├── capture/
│   │   ├── capture.go    # CaptureMode interface: BuildInputArgs(), StopProcess()
│   │   ├── decklink.go   # decklink mode: auto-detect, device name input args
│   │   ├── avfoundation.go # avfoundation fallback: manual format, index:index
│   │   ├── dshow.go      # dshow fallback: manual format, name-based
│   │   └── detect.go     # auto-mode resolution: probe ffmpeg, pick best mode
│   └── platform/
│       ├── platform.go   # interface for platform-specific behavior (stop method)
│       ├── darwin.go      # macOS: SIGINT stop
│       └── windows.go     # Windows: stdin 'q' stop
├── .goreleaser.yml        # cross-compilation config
├── README.md
└── Formula/
    └── osc-record.rb      # Homebrew formula template
```

## Dependencies (Go modules)

| Module | Purpose |
|--------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/BurntSushi/toml` | Config file parsing |
| `github.com/hypebeast/go-osc/osc` | OSC protocol server |

No other external dependencies. ffmpeg interaction is via `os/exec`.

---

## Homebrew Tap

Repository: `brodiegraphics/homebrew-tools`

Formula (`Formula/osc-record.rb`):

```ruby
class OscRecord < Formula
  desc "OSC-triggered video capture for live production"
  homepage "https://github.com/brodiegraphics/osc-record"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/brodiegraphics/osc-record/releases/download/v0.1.0/osc-record_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/brodiegraphics/osc-record/releases/download/v0.1.0/osc-record_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  # Standard ffmpeg works for avfoundation fallback.
  # For decklink mode (recommended with Blackmagic devices), install ffmpeg
  # with decklink support instead:
  #   brew tap homebrew-ffmpeg/ffmpeg
  #   brew install homebrew-ffmpeg/ffmpeg/ffmpeg --with-decklink
  depends_on "ffmpeg"

  def install
    bin.install "osc-record"
  end

  def caveats
    <<~EOS
      For Blackmagic capture devices, decklink mode is strongly recommended.
      It auto-detects signal format (resolution, framerate, pixel format).

      To enable decklink mode, install ffmpeg with decklink support:
        brew tap homebrew-ffmpeg/ffmpeg
        brew install homebrew-ffmpeg/ffmpeg/ffmpeg --with-decklink

      You also need the Blackmagic Desktop Video drivers installed:
        https://www.blackmagicdesign.com/support

      Without decklink support, osc-record falls back to avfoundation,
      which requires manual framerate and pixel format configuration.
    EOS
  end

  test do
    assert_match "osc-record v", shell_output("#{bin}/osc-record version")
  end
end
```

---

## GoReleaser Config

`.goreleaser.yml` for cross-platform builds:

```yaml
project_name: osc-record
builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -s -w -X main.version={{.Version}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

release:
  github:
    owner: brodiegraphics
    name: osc-record
```

---

## Error Messages

Keep error messages specific and actionable. Examples:

```
Error: ffmpeg not found on PATH. Install with 'brew install ffmpeg' or set ffmpeg.path in config.
Error: No record trigger configured. Run 'osc-record capture record' first.
Error: Could not bind to port 8000: address already in use. Use --port to specify a different port.
Error: Video device "Blackmagic UltraStudio Recorder" not found. Run 'osc-record devices' to list available devices.
Error: ffmpeg exited with code 1 during recording. Check that the capture device is receiving a valid signal.
Error: Output directory /Users/daniel/Dropbox/osc-record/ does not exist and could not be created: permission denied.
Warning: ffmpeg does not support decklink input format. Falling back to avfoundation. For auto-detect signal support, install ffmpeg with --with-decklink.
Warning: No valid signal detected on "UltraStudio Recorder". Recording will fail until a signal is present.
Error: Capture mode set to "decklink" but ffmpeg was not compiled with decklink support. Install ffmpeg with --with-decklink or set capture_mode to "auto".
```

---

## Future Considerations (Not In Scope for v0.1.0)

These are explicitly out of scope but worth noting so the architecture doesn't preclude them:

- Multiple simultaneous recordings (multiple camera inputs)
- Framerate/format override flags for fallback modes (currently hardcoded to 29.97/uyvy422)
- Web UI / status dashboard
- Custom ffmpeg argument passthrough
- Scheduled recording (time-based, not OSC-based)
- Metadata embedding in the output file (show name, date, etc.)
- Post-record webhook or email notification
