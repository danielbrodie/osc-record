# osc-record v0.2 — Build Plan

## Risks & Hard Problems First

Before sequencing, surface the three things that can go wrong and derail the build:

### Risk 1: Bubbletea message architecture
Bubbletea is a unidirectional loop: everything flows through `Update(msg)` → `View()`. Every async event — OSC packet received, signal poll result, ffmpeg crash, disk stat — must arrive as a `tea.Msg`. If this isn't designed correctly from the start, the whole model becomes untestable spaghetti. This is the most important decision in the build. We design all message types before writing a single panel.

### Risk 2: Device exclusivity
A decklink device can only be opened by one reader at a time. This creates a conflict:
- When IDLE: we want a background process reading the device to show signal status + audio levels
- When RECORDING: ffmpeg owns the device exclusively

Resolution: the idle monitor process is a goroutine that holds the device. When a record trigger arrives, we SIGTERM the monitor, wait for it to release the device (max 500ms), then start the recording process. When recording stops, we restart the monitor. The TUI model owns this state machine. Audio level data comes from the same monitor process when idle, and from the recording process's stderr when recording.

### Risk 3: Pre-roll buffer ↔ device exclusivity conflict
The spec says pre-roll uses an HLS ring buffer. But if the ring buffer ffmpeg process holds the device, nothing else can read it — including the recording process. Resolution: pre-roll operates differently depending on mode:

**With pre-roll enabled:** one persistent ffmpeg process writes rolling HLS segments AND is the recording process. On record trigger: redirect output from ring-buffer mode to normal mux by writing a sentinel to a named pipe / signaling ffmpeg to start a new segment file. This avoids the process-swap entirely.

Concretely: run ffmpeg with `-f segment -segment_time 1 -segment_wrap N` continuously. On trigger: stop the segment writer, use `ffmpeg -f concat` on the last N segments + a new live capture into the output file. Total downtime: ~500ms (acceptable). Trade-off documented.

**Without pre-roll:** idle monitor + recording process swap (Risk 2 resolution).

---

## Message Types

Define all `tea.Msg` types in `internal/tui/messages.go` before anything else:

```go
// Signal
SignalStateMsg       { Device, Input, Format, Resolution, FPS string; Locked bool }
AudioLevelMsg        { Left, Right float64 }   // dBFS, -inf to 0
TimecodeMsg          { TC string }

// OSC
OSCReceivedMsg       { Address, Args, Source string; Time time.Time }

// Recording
RecordingStartedMsg  { File string; Time time.Time }
RecordingStoppedMsg  { File string; Duration time.Duration; SizeBytes int64 }
RecordingCrashedMsg  { File string; Err error; Recoverable bool }
RecordingResumedMsg  {}

// Clips
ClipVerifiedMsg      { File string; OK bool; Errors []string }

// System
DiskStatMsg          { Path string; Free, Total uint64 }
ChecklistResultMsg   { Results []CheckResult }
ScanResultMsg        { Input, Format, Desc string; Locked bool; Preview []byte }
ScanDoneMsg          {}
TermSizeMsg          { Width, Height int }
TickMsg              { Time time.Time }   // 500ms blink tick
```

---

## Build Sequence

### Phase 0: Dependencies + scaffold
**~1 day**

Add bubbletea, lipgloss, bubbles, golang.org/x/term to go.mod. Create directory structure. Add `internal/tui/` package stub that compiles. Update `cmd/run.go` to detect TTY (`golang.org/x/term.IsTerminal(os.Stdout.Fd())`) and branch to TUI or plaintext. Keep v0.1 plaintext path 100% intact — it must still work throughout the entire build.

Files created: `internal/tui/messages.go`, `internal/tui/keys.go`, `internal/tui/styles.go`

**Milestone:** `go build ./...` passes with new deps. `osc-record run` still works in plaintext.

---

### Phase 1: Styles and layout skeleton
**~1 day**

`internal/tui/styles.go` — full lipgloss palette from spec. Every color constant defined once here. Border style (rounded), panel title style, status indicator styles.

`internal/tui/model.go` — bubbletea root model. No live data yet. Renders the four-panel layout with static placeholder text. Handles `TermSizeMsg` and enforces 100×30 minimum with resize prompt. Handles `Q` / `Ctrl+C`.

`internal/tui/keys.go` — key.Map for all shortcuts. No actions wired yet — just the map.

**Milestone:** `osc-record run` in a TTY shows the panel layout with placeholder text. Resize and quit work. No data.

---

### Phase 2: OSC monitor panel
**~1 day**

Why first: OSC is self-contained — no device, no ffmpeg, no conflicts. Best panel to validate the message → Update → View loop.

`internal/tui/oscmon.go` — bubbletea component wrapping a `viewport.Model`. Renders OSC entries; auto-scrolls when not manually scrolled. Highlights configured record/stop addresses in green/red.

Wire the OSC listener (from `internal/osc`) to send `OSCReceivedMsg` into the bubbletea program via `program.Send()`. The OSC goroutine stays the same as v0.1; it just calls `Send` instead of dispatching to a handler directly.

**Milestone:** Start the TUI, fire an OSC packet from QLab — it appears in the OSC panel in real time with timestamp and source IP.

---

### Phase 3: Signal panel — static
**~1 day**

`internal/tui/signal.go` — renders signal panel from model state. At this stage: shows device name, SDI/HDMI rows with locked/no-signal state (no audio yet, no timecode).

`internal/sigpoll/` — new package. `Poller` struct runs a goroutine that probes the device every 5 seconds via a 1-second ffmpeg call. Parses stderr for resolution/fps/lock status. Sends `SignalStateMsg` via `program.Send()`. Stops cleanly on `Close()`. When device is recording, the poller must be suspended (see Risk 2).

**Milestone:** Signal panel shows live lock/no-signal state, updates every 5 seconds.

---

### Phase 4: Recording integration
**~2 days**

This is the core of the app. The recording state machine lives in `internal/recorder/recorder.go` (extended from v0.1). A `Recorder` wraps an ffmpeg process and exposes `Start(file string)` / `Stop()` / `Events() <-chan RecorderEvent`. The TUI model calls these and receives events (started, stopped, crashed) converted to `tea.Msg`.

**State machine per device:**
```
IDLE → (record trigger) → STARTING → (ffmpeg ready) → RECORDING → (stop trigger) → STOPPING → IDLE
RECORDING → (ffmpeg crash) → RECOVERING → (probe ok) → IDLE | (probe fail) → ERROR
```

`internal/tui/status.go` — recording status panel. Reads from model state: current state, file, elapsed time (updated every second via `TickMsg`), file size (updated every 2s via `os.Stat`), disk free space (from `DiskStatMsg`).

`internal/diskmon/` — new package. Goroutine calls `unix.Statfs` / `windows.GetDiskFreeSpaceEx` every 10 seconds. Sends `DiskStatMsg`.

Wire `R` and `S` keyboard shortcuts as manual record/stop overrides.

**Milestone:** Full record/stop cycle via keyboard and OSC. Status panel shows live duration and file size. Disk space visible.

---

### Phase 5: Audio levels
**~1 day**

When IDLE: the signal poller runs a persistent ffmpeg with `-filter:a astats=metadata=1:reset=1 -f null -` and parses `lavfi.astats.Overall.RMS_level` from the output. Sends `AudioLevelMsg` at ~10fps.

When RECORDING: parse the same astats metadata from the recording ffmpeg's output. Add `-filter:a astats=metadata=1:reset=1` to the recording ffmpeg args; parse its stderr.

VU meter rendering in `signal.go`: horizontal bar using `█░` characters. Width = available panel width minus label. Color thresholds: green below -18dBFS, amber -18 to -6, red above -6.

**Milestone:** VU meters animate in signal panel during idle monitoring and recording.

---

### Phase 6: Clips panel + verification
**~1 day**

`internal/tui/clips.go` — list of clips this session. Updated on `RecordingStartedMsg` (add entry, status=in-progress), `RecordingStoppedMsg` (update duration/size), `ClipVerifiedMsg` (update status).

`internal/verifier/` — new package. After each stop, runs `ffprobe -v error -show_streams -show_format -of json {file}`. Checks: video stream present, audio stream present, duration within 2s of elapsed time, no corrupt codec tag. Sends `ClipVerifiedMsg`.

`V` key → frame preview of selected clip (first frame extraction, no device needed — from file).

**Milestone:** Clips panel populates per session. Each clip shows verified/failed status after stop.

---

### Phase 7: Pre-show checklist
**~1 day**

`internal/tui/checklist.go` — overlay component. Runs all checks concurrently on open, renders result list, shows fix hints for failures.

`internal/platform/darwin.go` — extend with `DriverStatus()` that runs `systemextensionsctl list` and parses for `BlackmagicIO.DExt` state.

Checks run as `tea.Cmd` functions that return `ChecklistResultMsg`.

Accessible via `F2`. Auto-runs at startup if record/stop addresses not configured.

**Milestone:** `F2` shows checklist overlay with live results. Driver check works on macOS.

---

### Phase 8: Setup wizard
**~2 days**

`internal/tui/wizard.go` — five-step wizard as a bubbletea component. Each step is a sub-model with its own `Update`/`View`. Steps:

1. Device picker table (calls `devices.ProbeForPlatform`)
2. Signal config — try autodetect, offer scanner if it fails
3. OSC address learning (embedded OSC monitor, same as v0.1 `capture record/stop` flow)
4. Output config (text inputs)
5. Checklist auto-run

Saves config to disk on completion.

`cmd/setup.go` — plaintext version. No bubbletea. Just runs the same logic in a readline loop. For scripted environments.

Remove `cmd/capture.go` (`capture record` / `capture stop` subcommands). BREAKING: document in release notes.

**Milestone:** First-run experience: open `osc-record run`, wizard appears, complete all steps, land on main TUI ready to record.

---

### Phase 9: Signal scanner
**~2 days**

`internal/tui/scanner.go` — overlay table component with bubbletea spinner.

`internal/scanner/` — new package. `Scan(device, ffmpegPath string) <-chan ScanResult`. For each `(video_input, format_code)` combination: run ffmpeg probe with 3s timeout, parse result. Channel yields results as they come. Sends `ScanResultMsg` per result, `ScanDoneMsg` when complete.

Format code list comes from running `ffmpeg -f decklink -list_formats 1 -i {device}` and parsing the output table.

Preview: for each locked result, run frame extraction (see Phase 10) and attach `[]byte` JPEG to `ScanResultMsg`.

**Milestone:** `F1` opens scanner, scans all formats, shows table with locked/no-signal per row, thumbnails for locked signals.

---

### Phase 10: Frame preview
**~1 day**

`internal/preview/capture.go` — `ExtractFrame(device, ffmpegPath, formatCode string) ([]byte, error)`. Runs ffmpeg, pipes one JPEG frame out via stdout.

`internal/preview/render.go` — `Render(jpeg []byte, width, height int) string`. Detects terminal:
- Check `$TERM_PROGRAM` for `iTerm.app` → use inline image protocol (`\033]1337;File=...`)
- Check `$TERM` for `xterm-kitty` → use Kitty graphics protocol
- Check sixel support via `\033[c` device attributes
- Fallback: decode JPEG, scale down, render as `▄▀` half-block art with ANSI 256-color

`internal/tui/preview.go` — overlay wrapper. `P` key grabs a frame from the live device; `V` key extracts from a clip file. Renders into a centered overlay panel.

**Milestone:** `P` in the TUI shows a frame preview of the live signal. `V` shows first frame of a completed clip.

---

### Phase 11: Pre-roll buffer
**~2 days**

`internal/preroll/` — new package.

`Preroller` struct manages the segment writer process. On `Start(device, ffmpegPath, formatCode string, seconds int)`: launches ffmpeg writing 1-second `.mp4` segments to a temp dir with `segment_wrap=N`. Maintains a ring of the last N segment paths.

On `Flush(outputFile string) error`: stops the segment writer, runs `ffmpeg -f concat` on the ring buffer segments + signals the main recorder to start. Returns when concat is complete and recorder has taken over the device.

The main `Recorder` in this mode does not do its own device probe — the preroller hands off cleanly.

On recorder `Stop()`: preroller restarts its segment writer for the next take.

If pre-roll is set: display `PRE-ROLL ACTIVE  {N}s buffer` in the status panel.

Edge cases:
- Segment writer crash: fall back to non-pre-roll recording, log event
- Buffer < N segments available (just started): flush whatever is there, note in log

**Milestone:** With `--pre-roll 5`, firing the record cue results in a file that begins 5 seconds before the trigger.

---

### Phase 12: Multi-device
**~2 days**

Config: parse `[[devices]]` array. Normalize single `[device]` block into a `[]DeviceConfig` slice internally.

`internal/recorder/recorder.go` — extend to manage N `DeviceRecorder` instances. `RecordAll()` / `StopAll()` fire in parallel goroutines. Each sends its own events tagged with device name.

TUI: Signal panel renders one row per device. Status panel renders one row per recording device. Clips panel groups by device.

**Milestone:** Two devices configured, single OSC trigger starts both simultaneously, two files saved.

---

### Phase 13: HTTP status server
**~0.5 days**

`internal/health/server.go` — `net/http` server. Three routes. Reads from a `StatusSnapshot` struct that the TUI model updates on every state change (mutex-protected). No external deps.

Wire `--http-port` flag. Log "HTTP status on :N" at startup.

**Milestone:** `curl localhost:8080/status` returns live JSON while recording.

---

### Phase 14: Show/scene/take slate
**~0.5 days**

Extend `RecordingConfig` with `Show`, `Scene`, `Take string`, `TakeAuto int`.

Filename builder: if all three set → `{show}-{scene}-{take}-{timestamp}.mp4`, else → legacy prefix format.

ffmpeg `-metadata` args: embed show/scene/take in output file.

`internal/tui/slate.go` — three-field text input overlay. `N` to open, Enter to save, Esc to cancel.

`T` key resets `TakeAuto` to 1.

**Milestone:** Set show/scene/take in TUI, record a clip, ffprobe shows metadata embedded.

---

### Phase 15: Post-show manifest
**~0.5 days**

`internal/manifest/manifest.go` — `Write(clips []ClipInfo, cfg Config, path string)`. Formats the text table and writes to disk.

`cmd/manifest.go` — `osc-record manifest [dir]` subcommand. Scans directory for `.mp4`/`.mov` files, runs ffprobe on each to extract metadata, generates manifest.

On TUI exit: if any clips were recorded this session, write manifest to output dir.

**Milestone:** After a session, a manifest `.txt` is in the output dir with all clip data.

---

### Phase 16: Confidence monitor
**~0.5 days**

`C` key: check if `ffplay` is in PATH, check macOS (skip on Windows with message), launch `exec.Command("ffplay", "-f", "decklink", "-format_code", code, "-i", device, "-fs", "-window_title", "osc-record confidence — "+device)`.

Monitor the process; if it exits early, log the event. Kill on TUI exit.

**Milestone:** `C` opens fullscreen ffplay window with live signal.

---

### Phase 17: Crash recovery hardening
**~1 day**

Already partially covered by the recording state machine (Phase 4). This phase adds:
- Exponential backoff on restart attempts (500ms, 1s, 2s, 4s, up to 10s)
- Maximum 3 restart attempts per recording session before entering ERROR state
- `RecoveryAttemptMsg` displayed in TUI log
- On recovery failure: amber banner persists until manually dismissed with `Esc`

**Milestone:** Kill the recording ffmpeg process manually mid-recording → TUI shows recovery attempt → restarts → returns to IDLE without crashing.

---

### Phase 18: Release
**~1 day**

- Update `.goreleaser.yml` with new version
- Update `Formula/osc-record.rb` with new SHA256s
- Update Homebrew tap
- Update README
- Write release notes (breaking: `capture record`/`capture stop` removed)
- Tag `v0.2.0` and push

---

## Delegation Strategy

v0.1 had repeated Codex failures. The fix:

**Write all interfaces and message types myself** (Phases 0-1). These define the contracts everything else plugs into. Getting these wrong is expensive.

**Delegate individual panels and packages to Codex**, one at a time, with:
- The exact file path
- The exact structs/interfaces it must implement
- The exact `tea.Msg` types it sends/receives
- The exact method signatures

Never say "implement the signal panel." Say "implement `internal/tui/signal.go` with a `SignalPanel` struct that has `Init() tea.Cmd`, `Update(msg tea.Msg) (SignalPanel, tea.Cmd)`, `View() string`, and handles `SignalStateMsg`, `AudioLevelMsg`, `TimecodeMsg`. Uses styles from `internal/tui/styles.go`. Renders exactly N lines regardless of content (fixed height)."

**Build phases in dependency order.** Never hand Codex a phase that depends on an unfinished interface.

**Verify after every file:** `go build ./...` and `go vet ./...` must pass before moving on.

---

## Total Estimate

| Phase | Days |
|-------|------|
| 0-1: Scaffold + styles | 2 |
| 2-3: OSC + signal panels | 2 |
| 4-5: Recording + audio | 3 |
| 6-7: Clips + checklist | 2 |
| 8: Setup wizard | 2 |
| 9-10: Scanner + preview | 3 |
| 11: Pre-roll | 2 |
| 12: Multi-device | 2 |
| 13-14: HTTP + slate | 1 |
| 15-16: Manifest + confidence | 1 |
| 17-18: Recovery + release | 2 |
| **Total** | **22** |

With parallel Codex delegation on independent panels: realistically **12-15 days** of wall time.

---

## Open Questions Before Starting

1. **Timecode:** decklink ffmpeg support for embedded LTC/VITC timecode via `-timecode_frame` or metadata? Need to test before committing to Phase 3.

2. **Pre-roll concat quality:** HLS segment boundaries may cause A/V sync issues at the join point. Need a test with the actual device before Phase 11.

3. **Multi-device conflict:** Can two decklink devices be open simultaneously by separate ffmpeg processes? Almost certainly yes (separate PCIe/Thunderbolt endpoints), but test before Phase 12.

4. **Sixel in Discord screenshots:** The frame preview sixel output won't render if Daniel screenshots the TUI in a non-sixel terminal. The fallback half-block art is the safe default for demos.
