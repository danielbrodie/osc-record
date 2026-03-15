# Brainstorm: v1.2.0 Auto-Detection

**Date:** 2026-03-14
**Status:** Ready for planning

## What We're Building

Zero-config startup for osc-record with Blackmagic DeckLink devices. A user plugs in a DeckLink card, runs `osc-record run`, and gets a working signal lock without specifying `video_input` or `format_code`. The system probes inputs and format codes automatically, persists the results, and shows clear progress in the TUI.

### Scope

1. **Auto-detect `video_input`** — probe HDMI then SDI; if both lock, prompt the user to choose
2. **Auto-detect `format_code`** — scan supported DeckLink format codes for the detected input, pick the one with a live signal (not color bars)
3. **Persist discovered config** — write both values to `config.toml` atomically after the full detection sequence completes
4. **Setup wizard: `video_input` step** — add input selection (HDMI / SDI / Auto-detect) to `osc-record setup`
5. **Signal panel: "probing..." state** — show `⟳ probing...` in the TUI signal panel while detection runs in the background

## Why This Approach

**Motivation:** The 360stagedesign onboarding required 6 reinstalls, remote SSH debugging, manual format code scanning, and direct config editing. Every failure was caused by the absence of auto-detection.

**Design philosophy:** Zero interaction on the happy path (single input, one format match). Interaction only when ambiguity exists (both inputs have signal). Background probing keeps the TUI responsive and avoids false "no signal" alarms during the first seconds of startup.

## Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Both inputs have signal | Prompt user to choose | Avoids silently picking the wrong input when both are connected |
| Probe UX | Background with async tea.Msg | TUI stays interactive during probing; fits existing bubbletea patterns |
| Shared logic location | Expand `scanner` package | Already has format-code scan loop; add input probing there instead of a new package |
| Config persistence timing | Atomic write after full sequence | No partial config states; both `video_input` and `format_code` written together |
| Probe order | HDMI first, then SDI | HDMI is the more common input for typical camera setups |
| Color bars handling | Reject as "no signal" | Existing `sigpoll` already distinguishes color bars from live signal |

## Design Notes

### Auto-detect flow (startup)

```
config.toml has video_input + format_code?
  ├─ YES → skip auto-detect, use saved values
  └─ NO →
       1. Signal panel shows "⟳ probing..."
       2. Probe HDMI → lock?
       3. Probe SDI → lock?
       4. If both lock → prompt user to pick
       5. If one locks → use it
       6. If neither → show "no signal" (existing behavior)
       7. Scan format codes for chosen input (reuse scanner logic)
       8. Write video_input + format_code to config.toml atomically
       9. Signal panel updates to normal locked state
```

### Device contention

Auto-detect probes must respect the existing contention protocol: `poller.Suspend()` and `aMeter.Stop()` before opening the DeckLink device, `poller.Resume()` / `startAudioMeter()` after. The scanner package already handles this via `SetProbeHooks`.

### Setup wizard addition

Add a step between device selection and the existing flow:
- "What input is your camera connected to? [HDMI / SDI / Auto-detect]"
- Default: Auto-detect
- If Auto-detect is chosen, the values are left empty in config so startup auto-detection triggers

### Re-triggering detection

User clears `video_input` and/or `format_code` from `config.toml` (or deletes the keys) to re-trigger auto-detection on next run. No special CLI flag needed.

## Open Questions

_None — all key decisions resolved during brainstorming._
