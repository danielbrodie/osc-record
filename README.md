# osc-record

OSC-triggered video capture for live production. Send an OSC message from QLab, Disguise, a lighting console, or any OSC source — recording starts. Send another — it stops. Files land wherever you point it.

Built for theatrical environments where you need frame-accurate, hands-free recording triggered by show control.

## Install

```
brew tap danielbrodie/tap
brew install osc-record
```

> **Blackmagic device?** For decklink capture (recommended), you need ffmpeg compiled with decklink support and the [Blackmagic Desktop Video](https://www.blackmagicdesign.com/support) drivers installed. Without decklink, osc-record falls back to avfoundation/dshow which requires manual format configuration.
>
> decklink-enabled ffmpeg:
> ```
> brew tap homebrew-ffmpeg/ffmpeg
> brew install homebrew-ffmpeg/ffmpeg/ffmpeg --with-decklink
> ```

## Quick Start

**1. Learn your OSC triggers:**
```
osc-record capture record   # fire your record cue, press Enter to save
osc-record capture stop     # fire your stop cue, press Enter to save
```

**2. Run:**
```
osc-record run
```

On first run, osc-record probes for capture devices and shows a picker if `device.name` isn't set in config. Select your device — it's saved automatically.

**3. Trigger from QLab / Disguise / console:**

Send the OSC address you learned in step 1 to `<felix-ip>:8000`. Recording starts. Send the stop address — file is saved.

## Commands

| Command | Description |
|---------|-------------|
| `osc-record run` | Start the daemon — listens for OSC, records on trigger |
| `osc-record devices` | List available capture devices |
| `osc-record capture record` | Interactively learn the record OSC address |
| `osc-record capture stop` | Interactively learn the stop OSC address |
| `osc-record version` | Print version |

### `run` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 8000 | OSC listen port |
| `--output` | `~/Dropbox/osc-record` | Output directory |
| `--prefix` | `recording` | Filename prefix |
| `--profile` | `h264` | Encoding profile: `h264`, `hevc`, `prores` |
| `--video-device` | from config | Override video device |
| `--capture-mode` | `auto` | `auto`, `decklink`, `avfoundation`, `dshow` |
| `--config` | `~/.config/osc-record/config.toml` | Config file path |

## Config

Config lives at `~/.config/osc-record/config.toml` (macOS/Linux) or `%APPDATA%\osc-record\config.toml` (Windows). Auto-created on first run.

```toml
[osc]
  port = 8000
  record_address = "/record/start"
  stop_address = "/record/stop"

[device]
  capture_mode = "auto"       # auto, decklink, avfoundation, dshow
  name = "UltraStudio Recorder 3G"
  audio = ""                  # avfoundation/dshow only
  format_code = "Hp59"        # decklink only — leave empty to autodetect
                              # run: ffmpeg -f decklink -list_formats 1 -i "Device Name"

[recording]
  profile = "h264"            # h264, hevc, prores
  prefix = "recording"
  output_dir = "~/Dropbox/osc-record/"

[ffmpeg]
  path = ""                   # leave empty to use $PATH
```

Legacy single-device config stays valid. For synchronized multi-device recording, use `[[devices]]` instead of `[device]`:

```toml
[osc]
  port = 8000
  record_address = "/record/start"
  stop_address = "/record/stop"

[[devices]]
  capture_mode = "decklink"
  name = "UltraStudio Recorder 3G"
  format_code = "Hp59"

[[devices]]
  capture_mode = "decklink"
  name = "UltraStudio 4K Mini"
  format_code = "Hp59"

[recording]
  profile = "h264"
  prefix = "TEST"
  output_dir = "~/Dropbox/osc-record/"
```

### `device.format_code`

Most decklink devices autodetect the incoming signal format. Some (including the UltraStudio Recorder 3G over SDI) do not. If you get a "Cannot Autodetect input stream" error, set `format_code` to the matching code for your signal.

List supported codes for your device:
```
ffmpeg -f decklink -list_formats 1 -i "Your Device Name"
```

Common codes: `Hp59` (1080p59.94), `Hp29` (1080p29.97), `Hi59` (1080i59.94), `23ps` (1080p23.976)

## Output Files

Files are named `{prefix}-{YYYY-MM-DD-HHmmss}.{ext}` and saved to `output_dir`. With multi-device config, osc-record appends the device name to the prefix so one trigger produces one file per device.

## Capture Modes

**decklink** (recommended with Blackmagic devices): Uses ffmpeg's native DeckLink integration. Auto-detects signal format, embeds audio from SDI. Requires ffmpeg built with `--enable-decklink` and Blackmagic Desktop Video drivers.

**avfoundation** (macOS fallback): Uses macOS's AVFoundation framework. Works without special ffmpeg builds. Requires manual format configuration if signal doesn't match defaults.

**dshow** (Windows fallback): DirectShow equivalent of avfoundation.

`auto` mode tries decklink first and falls back automatically.

## Requirements

- macOS 12+ or Windows 10+
- ffmpeg in `$PATH` (or set `ffmpeg.path` in config)
- For decklink: Blackmagic Desktop Video 14.3+, ffmpeg with `--enable-decklink`

## License

MIT
