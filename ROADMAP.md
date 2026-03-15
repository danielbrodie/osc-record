# osc-record Roadmap

## v1.2.0 — Auto-Detection

### Goal
A user should be able to plug in a Blackmagic DeckLink device and run `osc-record run` with zero manual config and get a working signal lock. No format codes, no video_input guessing.

### Features

**1. Auto-detect `video_input`**
On startup, if `device.video_input` is empty or `"auto"`, probe both `hdmi` and `sdi` inputs sequentially. First one that locks wins. Save result to `config.toml`.

**2. Auto-detect `format_code`**
On startup, if `device.format_code` is empty, scan all supported DeckLink format codes for the detected input. Pick the one that locks with a live signal (not color bars). Save result to `config.toml`.

Uses the same logic as the existing `[F1] Scan` TUI action — extract into a shared function.

**3. Persist discovered config**
Write both `video_input` and `format_code` back to `config.toml` immediately after detection. On subsequent runs, skip auto-detect and use saved values. User can clear them to re-trigger detection.

**4. Setup wizard: add `video_input` step**
The `osc-record setup` flow currently skips input selection. Add a step: "What input is your camera connected to? [HDMI / SDI / Auto-detect]". Default: Auto-detect.

**5. Signal panel: "probing..." state**
Show a `⟳ probing...` indicator during the auto-detect scan instead of jumping straight to "no signal". Prevents false alarms during the first 5–10 seconds of startup.

### Motivation
The 360stagedesign onboarding required: 6 reinstalls, remote SSH debugging, manual format code scanning, and direct config editing. Every step was caused by the absence of auto-detection. This eliminates the entire class of "no signal" false failures on first run.

### Non-Goals
- No changes to recording behavior
- No new OSC addresses
- No UI layout changes
- `[F1] Scan` remains as a manual override

---

## Backlog

- **amd64 bottle for `ffmpeg-decklink`**: needs Intel Mac or GitHub Actions runner with DeckLink SDK
- **Audio meter CPU optimization**: currently ~29% CPU on Apple Silicon; explore lower-rate polling or lighter astats config
- **Windows `ffprobe-decklink` fallback**: `ffprobe-decklink.exe` equivalent not packaged yet
- **Multi-device auto-detection**: v1.2.0 scope covers single device only; `[[devices]]` array auto-detect is future work
