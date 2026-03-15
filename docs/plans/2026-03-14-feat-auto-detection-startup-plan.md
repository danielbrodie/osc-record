---
title: "feat: Auto-detect video input and format code on startup"
type: feat
status: completed
date: 2026-03-14
origin: docs/brainstorms/2026-03-14-auto-detection-brainstorm.md
---

# feat: Auto-detect video input and format code on startup

## Overview

Eliminate manual `video_input` and `format_code` configuration for Blackmagic DeckLink devices. On first run, osc-record probes HDMI and SDI inputs, scans format codes for the locked input, persists both values to `config.toml`, and shows progress in the TUI signal panel. Subsequent runs skip detection and use saved values.

## Problem Statement / Motivation

The 360stagedesign onboarding required 6 reinstalls, remote SSH debugging, manual format code scanning, and direct config editing — all caused by the absence of auto-detection. This feature eliminates the entire class of "no signal" false failures on first run.

(see brainstorm: `docs/brainstorms/2026-03-14-auto-detection-brainstorm.md`)

## Proposed Solution

Five coordinated changes (see brainstorm for decision rationale):

1. **Auto-detect `video_input`** — probe HDMI then SDI; if both lock, prompt user to choose via TUI overlay or plaintext prompt
2. **Auto-detect `format_code`** — scan format codes for the detected input using expanded `scanner` package; reject color-bars hits
3. **Persist config** — write both values atomically (write-to-tmp-then-rename) after the full sequence completes
4. **Setup wizard step** — add video input selection to both `osc-record setup` (CLI) and TUI wizard (W key)
5. **Signal panel "probing..." state** — background indicator via new `Probing` field on `SignalStateMsg`

## Technical Approach

### Resolved Questions

These gaps were identified during spec-flow analysis. Resolutions documented here to unblock implementation:

| Question | Resolution |
|---|---|
| Skip condition semantics | `""` and `"auto"` both treated as absent, checked independently per field. If `video_input` is known but `format_code` is absent → format scan only |
| OSC record during detection | Record trigger is queued. Detection aborts via context cancellation. Recording starts with whatever config exists (may fail if no format_code) |
| Config atomicity | Change `config.Save()` to write-to-tmp-then-rename. On write failure, keep discovered values in memory for the session; show error banner |
| Disambiguation UI | New `InputChoiceOverlay` (TUI) or numbered prompt (plaintext) |
| Plaintext mode support | Auto-detect runs in both TUI and plaintext paths. Plaintext prints "Probing inputs..." and blocks |
| Full sequence timeout | 90-second hard limit. On expiry, use best partial result or fall back to "no signal" |
| Color-bars in scanner | Add color-bars rejection to `scanner.probeFormat()` — parse stderr for "No input signal detected" |
| F1 during auto-detect | Disable F1 (scan) while auto-detect is running |
| Post-detection feedback | Log entry: `"✓ Auto-detected: HDMI, 1920x1080 59.94fps (Hp59) — saved to config"` |
| Multi-device configs | Auto-detect scoped to single-device configs only. `[[devices]]` array configs skip auto-detect with a log warning |

### Implementation Phases

#### Phase 1: Foundation — scanner expansion + color-bars fix

**Goal:** Make the scanner package capable of probing video inputs and rejecting color-bars, without changing any startup behavior yet.

**Files:**
- `internal/scanner/scanner.go` — expand with input probing + color-bars detection
- `internal/config/config.go` — atomic save (write-tmp-rename)
- `internal/tui/messages.go` — new message types
- `internal/tui/scanner.go` — add `ColorBars` field to `ScanResultEntry`

**Tasks:**

1.1. **Add `ColorBars` detection to `scanner.probeFormat()`** (`scanner.go:65`)
   - Parse stderr output for `"No input signal detected"` (same logic as `sigpoll.probe()`)
   - Add `ColorBars bool` field to `tui.ScanResultEntry`
   - Set `Locked = err == nil && !colorBars`
   - This fixes a pre-existing false-positive bug in the F1 manual scan

1.2. **Add `ProbeInput()` function to scanner package** (`scanner.go`)
   ```go
   // ProbeInput probes a specific video input (hdmi/sdi) for signal lock.
   // Returns true if a live signal (not color bars) is detected.
   func ProbeInput(ctx context.Context, ffmpegPath, device, videoInput string) (locked bool, err error)
   ```
   - Uses a 4-second timeout (matches `sigpoll` probe duration)
   - Runs a short ffmpeg capture: `-f decklink -video_input <input> -i <device> -t 1 -f null -`
   - Checks exit code + color-bars string
   - Does NOT hold any poller/meter locks — caller is responsible

1.3. **Add `AutoDetect()` orchestrator to scanner package** (`scanner.go`)
   ```go
   type AutoDetectResult struct {
       VideoInput string           // "hdmi" or "sdi"
       FormatCode string           // e.g. "Hp59"
       FormatDesc string           // e.g. "1080p 59.94"
       BothLocked bool             // true if both inputs had signal
   }

   // AutoDetect probes video inputs and scans format codes.
   // send receives progress messages for the TUI.
   // If both inputs lock, returns BothLocked=true with no VideoInput chosen — caller must disambiguate.
   func AutoDetect(ctx context.Context, ffmpegPath, device string, send func(AutoDetectProgressMsg)) (*AutoDetectResult, error)
   ```
   - Calls `ProbeInput("hdmi")`, then `ProbeInput("sdi")`
   - If both lock, returns `BothLocked: true` without choosing
   - If one locks, runs `Run()` (format scan) for that input, filters out `ColorBars` entries
   - Picks first locked, non-color-bars format code
   - Sends progress messages throughout

1.4. **Add new message types** (`tui/messages.go`)
   ```go
   type AutoDetectProgressMsg struct {
       Phase   string  // "input-probe", "format-scan", "complete", "failed"
       Detail  string  // human-readable status, e.g. "Probing HDMI..."
   }

   type AutoDetectCompleteMsg struct {
       Result *scanner.AutoDetectResult
       Err    error
   }

   type InputChosenMsg struct {
       VideoInput string  // "hdmi" or "sdi"
   }
   ```

1.5. **Make `config.Save()` atomic** (`config.go:133`)
   - Write to `<path>.tmp`, then `os.Rename()` to `<path>`
   - This prevents truncated TOML on crash during write

**Success criteria:**
- `scanner.ProbeInput()` correctly detects signal on a connected input
- `scanner.AutoDetect()` returns correct `VideoInput` + `FormatCode` for a single-input setup
- Color-bars hits are rejected in both `ProbeInput` and format scan
- `config.Save()` is crash-safe
- Existing F1 scan behavior unchanged (but now rejects color-bars)

---

#### Phase 2: TUI integration — probing state + disambiguation overlay

**Goal:** Wire auto-detection into the TUI startup path with visual feedback.

**Files:**
- `internal/tui/signal.go` — probing state rendering
- `internal/tui/messages.go` — `Probing` field on `SignalStateMsg`
- `internal/tui/model.go` — handle new messages, manage auto-detect state
- `internal/tui/inputchoice.go` — new overlay for disambiguation
- `internal/tui/commands.go` — no new commands needed (auto-triggered)
- `cmd/run.go` — auto-detect goroutine in startup sequence

**Tasks:**

2.1. **Add `Probing` field to `SignalStateMsg`** (`tui/messages.go:14`)
   - `Probing bool` — when true, signal panel shows `⟳ probing...` instead of lock/error state

2.2. **Update signal panel rendering** (`tui/signal.go`)
   - Add `probing bool` field to `SignalPanel` struct
   - In `Update()`: set `p.probing = msg.Probing`
   - In `inputRow()`: if `p.probing`, render `⟳` indicator with amber styling and `"probing..."` text
   - Probing state takes precedence over `colorBars` and `locked` checks

2.3. **Create `InputChoiceOverlay`** (`tui/inputchoice.go` — new file)
   - Implements the existing `Overlay` interface
   - Shows: "Both HDMI and SDI have signal. Which input is your camera connected to?"
   - Two options: HDMI, SDI (up/down + enter to select)
   - On selection, sends `InputChosenMsg` via tea.Cmd
   - Styled consistently with existing overlays (scanner, wizard)
   - Compact: ~100 lines

2.4. **Handle auto-detect lifecycle in `model.go`**
   - On `AutoDetectProgressMsg`: update signal panel probing state, add log entries
   - On `AutoDetectCompleteMsg`:
     - If `result.BothLocked`: open `InputChoiceOverlay`
     - If success: clear probing state, log result, send `ConfigUpdatedMsg`
     - If error: clear probing state, log error, fall through to normal "no signal"
   - On `InputChosenMsg`: resume detection with chosen input (send back to `cmd/run.go` via channel)
   - Add `autoDetecting bool` to `Model` — gates F1 scan (disable while true)

2.5. **Wire auto-detect goroutine in `cmd/run.go`**
   - Insert between poller start (line 564) and runner goroutine (line 625)
   - Guard clause: skip if not decklink mode, skip if multi-device config, skip if both `video_input` and `format_code` are set and not `"auto"`
   - Flow:
     1. Send `SignalStateMsg{Probing: true}` to TUI
     2. `poller.Suspend()` + `stopAudioMeter()`
     3. Call `scanner.AutoDetect(ctx, ...)`
     4. If `BothLocked`: send `AutoDetectCompleteMsg{BothLocked}`, wait on `inputChoiceCh` for user selection, then continue format scan
     5. Update config: `devices[0].VideoInput`, `devices[0].FormatCode`
     6. `cfg.SetDevices(devices, ...)` + `saveConfig(cfg)`
     7. Restart poller with new values + `startAudioMeter()`
     8. Send `SignalStateMsg{Probing: false, Locked: true, ...}` with detected values
   - Coordinate with 4-second audio meter delay: don't start audio meter timer until after auto-detect completes
   - Context with 90-second timeout
   - If OSC record arrives during detection: cancel detection context, let record proceed

2.6. **Add `ConfigUpdatedMsg` handling** (`tui/model.go`)
   - Refresh the status panel's device/format display after auto-detect writes config
   - `type ConfigUpdatedMsg struct { VideoInput, FormatCode string }`

**Success criteria:**
- Signal panel shows `⟳ probing...` during detection, transitions to locked state after
- When both inputs lock, overlay appears and user can choose
- F1 is disabled during auto-detect
- OSC record trigger cancels detection and starts recording
- Config is written and poller restarted with detected values
- Status panel reflects detected format code

---

#### Phase 3: Setup wizard + plaintext mode

**Goal:** Complete the feature across all user-facing paths.

**Files:**
- `cmd/setup.go` — new video input step
- `internal/tui/wizard.go` — new wizard step
- `cmd/run.go` — plaintext auto-detect path

**Tasks:**

3.1. **Add video input step to CLI setup** (`cmd/setup.go`)
   - Insert after existing OSC stop address step, before save
   - Prompt: `"Video input [1=HDMI / 2=SDI / 3=Auto-detect (default)]: "`
   - Default: Auto-detect (press enter)
   - If HDMI or SDI: set `cfg.Device.VideoInput = "hdmi"/"sdi"`
   - If Auto-detect: leave `cfg.Device.VideoInput = ""` (triggers auto-detect on `run`)

3.2. **Add `WizardStepVideoInput` to TUI wizard** (`tui/wizard.go`)
   - New step constant between `WizardStepDevice` and `WizardStepOSCRecord`
   - Add `VideoInput string` field to `WizardResult`
   - Three options: HDMI, SDI, Auto-detect
   - Update step labels array for progress display
   - Handle in `advance()` and `View()`

3.3. **Handle `WizardDoneMsg` for `VideoInput`** (`cmd/run.go`)
   - In the existing `WizardDoneMsg` handler, write `result.VideoInput` to config alongside device name
   - If `VideoInput` is empty (auto-detect), trigger auto-detect sequence

3.4. **Plaintext auto-detect path** (`cmd/run.go` in `runPlaintext`)
   - Same guard clause as TUI path
   - Print `"Probing video inputs..."` to stdout
   - Run `scanner.AutoDetect()` synchronously (blocking)
   - If both lock: numbered prompt `"Both HDMI and SDI have signal.\n  1. HDMI\n  2. SDI\nChoose [1]: "`
   - Print result: `"Auto-detected: HDMI, 1080p 59.94fps (Hp59)"`
   - Write config and continue startup

**Success criteria:**
- `osc-record setup` includes video input selection step
- TUI wizard (W key) includes video input step
- Plaintext/headless mode auto-detects with blocking prompts
- All paths write consistent config

---

### Architecture

```
cmd/run.go (startup orchestrator)
    │
    ├── guard clause: needsAutoDetect(cfg, mode)
    │     └── checks: decklink mode, single device, video_input/format_code empty or "auto"
    │
    ├── TUI path: autoDetect goroutine
    │     ├── scanner.AutoDetect() ──────────────────────┐
    │     │     ├── scanner.ProbeInput("hdmi")            │
    │     │     ├── scanner.ProbeInput("sdi")             │  internal/scanner/scanner.go
    │     │     └── scanner.Run() (format scan)           │  (expanded package)
    │     │           └── scanner.probeFormat() ──────────┘
    │     │                 └── color-bars check (shared with sigpoll)
    │     │
    │     ├── sends: AutoDetectProgressMsg → tui.Model → SignalPanel ("⟳ probing...")
    │     ├── sends: AutoDetectCompleteMsg → tui.Model
    │     │     └── if BothLocked → InputChoiceOverlay → InputChosenMsg
    │     │
    │     └── config.Save() (atomic write-tmp-rename)
    │
    └── Plaintext path: blocking auto-detect
          └── same scanner.AutoDetect() + fmt.Printf prompts
```

## System-Wide Impact

- **Device contention:** Auto-detect holds the DeckLink device for 10–90 seconds during startup. The poller is suspended and audio meter delayed for the full duration. Any OSC record trigger cancels detection via context.
- **Error propagation:** Detection failure is non-fatal — falls through to existing "no signal" state. Config write failure is logged but session continues with in-memory values.
- **State lifecycle:** No risk of orphaned state. Config is written atomically or not at all. In-memory device config is updated before poller restart.
- **API surface parity:** The `/status` HTTP endpoint already reflects `SignalStateMsg` — it will show `probing: true` during detection with no changes needed.

## Acceptance Criteria

### Functional Requirements

- [x] Running `osc-record run` with no `video_input` or `format_code` in config triggers auto-detection
- [x] Auto-detection probes HDMI then SDI; first lock wins (single input case)
- [x] When both inputs lock, user is prompted to choose (TUI overlay or plaintext prompt)
- [x] Format code scan runs for the detected input; color-bars results are rejected
- [x] Discovered `video_input` and `format_code` are written to `config.toml` after the full sequence
- [x] Subsequent runs with saved config skip auto-detection entirely
- [x] Clearing `video_input`/`format_code` from config re-triggers detection on next run
- [x] `video_input = "auto"` is treated as absent (triggers detection)
- [x] Signal panel shows `⟳ probing...` during detection, transitions to locked state after
- [x] F1 scan is disabled during auto-detect
- [x] `osc-record setup` includes a video input selection step (HDMI / SDI / Auto-detect)
- [x] TUI wizard (W key) includes a video input selection step
- [x] Plaintext/headless mode supports auto-detect with blocking prompts
- [x] Multi-device `[[devices]]` configs skip auto-detect with a log warning
- [x] OSC record trigger during detection cancels detection and starts recording
- [x] `config.Save()` uses write-to-tmp-then-rename for crash safety

### Non-Functional Requirements

- [x] Full detection sequence completes in under 90 seconds
- [x] No changes to recording behavior
- [x] No new OSC addresses
- [x] No UI layout changes beyond the probing indicator
- [x] F1 manual scan remains as a manual override (now also rejects color-bars)

## Dependencies & Risks

| Risk | Mitigation |
|---|---|
| Detection holds device for up to 90s, blocking recording | Context cancellation on OSC record trigger; 90s hard timeout |
| Color-bars false positive in format scan | Add color-bars check to `scanner.probeFormat()` (Phase 1) |
| Config write corruption on crash | Atomic write-to-tmp-then-rename (Phase 1) |
| Audio meter fires during detection | Delay audio meter start until after detection completes |
| F1 scan during auto-detect | Disable F1 while `model.autoDetecting` is true |
| Wizard writes config during detection | Auto-detect goroutine checks for config changes before writing |

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-14-auto-detection-brainstorm.md](docs/brainstorms/2026-03-14-auto-detection-brainstorm.md) — Key decisions: prompt user when both inputs lock, background probe with async tea.Msg, expand scanner package, atomic config write after full sequence.

### Internal References

- Scanner package: `internal/scanner/scanner.go` — format code scanning, `probeFormat()`, `Run()`
- Signal poller: `internal/sigpoll/poller.go` — `Suspend()`/`Resume()`, color-bars detection in `probe()`
- Config I/O: `internal/config/config.go` — `Save()`, `SetDevices()`, `ActiveDevices()`
- TUI signal panel: `internal/tui/signal.go` — `inputRow()` rendering
- TUI messages: `internal/tui/messages.go` — `SignalStateMsg`, `ScanResultEntry`
- Scanner overlay: `internal/tui/scanner.go` — overlay pattern to follow for `InputChoiceOverlay`
- Wizard: `internal/tui/wizard.go` — `WizardStep` enum, `WizardResult` struct
- CLI setup: `cmd/setup.go` — plaintext wizard flow
- Startup sequence: `cmd/run.go:397` — `runTUI`, poller start, runner goroutine
- Audio meter: `internal/audiometer/meter.go` — `Stop()` 300ms sleep for device release
