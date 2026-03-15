package scanner

import (
	"context"
	"fmt"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

// AutoDetectResult holds the outcome of the auto-detection sequence.
type AutoDetectResult struct {
	VideoInput string // "hdmi" or "sdi"
	FormatCode string // e.g. "Hp59"
	FormatDesc string // e.g. "1080p 59.94fps"
	BothLocked bool   // true if both inputs had live signal
}

// AutoDetect probes video inputs and scans format codes for the first input
// that locks. If both inputs lock, it returns BothLocked=true with no
// VideoInput chosen — the caller must disambiguate and call
// AutoDetectFormat separately.
//
// The caller is responsible for suspending any competing device access
// (poller, audio meter) before calling this function.
func AutoDetect(ctx context.Context, ffmpegPath, device string, send func(tui.AutoDetectProgressMsg)) (*AutoDetectResult, error) {
	send(tui.AutoDetectProgressMsg{Phase: "input-probe", Detail: "Probing HDMI..."})

	hdmiLocked, err := ProbeInput(ctx, ffmpegPath, device, "hdmi")
	if err != nil {
		return nil, fmt.Errorf("HDMI probe failed: %w", err)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	send(tui.AutoDetectProgressMsg{Phase: "input-probe", Detail: "Probing SDI..."})

	sdiLocked, err := ProbeInput(ctx, ffmpegPath, device, "sdi")
	if err != nil {
		return nil, fmt.Errorf("SDI probe failed: %w", err)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Both locked — caller must disambiguate.
	if hdmiLocked && sdiLocked {
		send(tui.AutoDetectProgressMsg{Phase: "input-probe", Detail: "Both HDMI and SDI have signal"})
		return &AutoDetectResult{BothLocked: true}, nil
	}

	var chosenInput string
	switch {
	case hdmiLocked:
		chosenInput = "hdmi"
	case sdiLocked:
		chosenInput = "sdi"
	default:
		send(tui.AutoDetectProgressMsg{Phase: "failed", Detail: "No signal on HDMI or SDI"})
		return nil, fmt.Errorf("no signal detected on HDMI or SDI")
	}

	result, err := AutoDetectFormat(ctx, ffmpegPath, device, chosenInput, send)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AutoDetectFormat runs the format code scan for a specific input and returns
// the first format with a live signal. Call this after disambiguating when
// BothLocked is true.
func AutoDetectFormat(ctx context.Context, ffmpegPath, device, videoInput string, send func(tui.AutoDetectProgressMsg)) (*AutoDetectResult, error) {
	send(tui.AutoDetectProgressMsg{
		Phase:  "format-scan",
		Detail: fmt.Sprintf("Scanning format codes on %s...", videoInput),
	})

	// Use a generous timeout for the full scan (16 formats × 3s each = 48s max).
	scanCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	results := Run(scanCtx, ffmpegPath, device, videoInput, func(msg tui.ScanProgressMsg) {
		send(tui.AutoDetectProgressMsg{
			Phase:  "format-scan",
			Detail: fmt.Sprintf("Probing %s (%d/%d)", msg.Current, msg.Done, msg.Total),
		})
	})

	// Pick the first format with a live signal (not color bars).
	for _, r := range results {
		if r.Locked && !r.ColorBars {
			send(tui.AutoDetectProgressMsg{
				Phase:  "complete",
				Detail: fmt.Sprintf("Detected: %s %s (%s)", videoInput, r.Description, r.FormatCode),
			})
			return &AutoDetectResult{
				VideoInput: videoInput,
				FormatCode: r.FormatCode,
				FormatDesc: r.Description,
			}, nil
		}
	}

	send(tui.AutoDetectProgressMsg{Phase: "failed", Detail: fmt.Sprintf("No format locked on %s", videoInput)})
	return nil, fmt.Errorf("no format code locked with live signal on %s", videoInput)
}
