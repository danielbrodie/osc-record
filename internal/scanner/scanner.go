// Package scanner probes all supported decklink format codes and reports which have signal.
package scanner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

// FormatEntry is a decklink format code with a human-readable description.
type FormatEntry struct {
	Code string
	Desc string
}

// formatLineRe matches ffmpeg -list_formats output lines like:
//
//	ntsc		720x486 at 30000/1001 fps (interlaced, lower field first)
var formatLineRe = regexp.MustCompile(`^\t(\S+)\s+(.+)$`)

// QueryFormats asks ffmpeg for the format codes supported by a DeckLink device.
// Falls back to a minimal hardcoded list if the query fails.
func QueryFormats(ffmpegPath, device string) []FormatEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, "-f", "decklink", "-list_formats", "1", "-i", device)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // always exits non-zero — output is on stderr

	var formats []FormatEntry
	for _, line := range strings.Split(stderr.String(), "\n") {
		m := formatLineRe.FindStringSubmatch(line)
		if m == nil || m[1] == "format_code" {
			continue
		}
		formats = append(formats, FormatEntry{
			Code: m[1],
			Desc: humanDesc(m[1], m[2]),
		})
	}
	if len(formats) == 0 {
		return fallbackFormats
	}
	return formats
}

// humanDesc converts ffmpeg's raw description into a shorter, friendlier form.
// e.g. "1920x1080 at 24000/1000 fps" → "1920x1080 24fps"
//
//	"720x486 at 30000/1001 fps (interlaced, lower field first)" → "720x486 29.97fps interlaced"
func humanDesc(code, raw string) string {
	raw = strings.TrimSpace(raw)

	// Extract resolution
	res := ""
	if i := strings.Index(raw, " at "); i > 0 {
		res = raw[:i]
	}

	// Extract fps fraction and convert to decimal
	fps := ""
	if i := strings.Index(raw, " at "); i >= 0 {
		rest := raw[i+4:]
		if j := strings.Index(rest, " fps"); j > 0 {
			frac := rest[:j]
			fps = fracToFPS(frac)
		}
	}

	interlaced := strings.Contains(raw, "interlaced")

	desc := res
	if fps != "" {
		desc += " " + fps + "fps"
	}
	if interlaced {
		desc += " interlaced"
	}
	return desc
}

// fracToFPS converts "24000/1000" → "24", "24000/1001" → "23.976", etc.
func fracToFPS(frac string) string {
	parts := strings.SplitN(frac, "/", 2)
	if len(parts) != 2 {
		return frac
	}
	var num, den float64
	fmt.Sscanf(parts[0], "%f", &num)
	fmt.Sscanf(parts[1], "%f", &den)
	if den == 0 {
		return frac
	}
	fps := num / den
	// Clean up common values
	switch {
	case fps > 59.9 && fps < 60.1:
		if den == 1001 {
			return "59.94"
		}
		return "60"
	case fps > 49.9 && fps < 50.1:
		return "50"
	case fps > 47.9 && fps < 48.1:
		if den == 1001 {
			return "47.95"
		}
		return "48"
	case fps > 29.9 && fps < 30.1:
		if den == 1001 {
			return "29.97"
		}
		return "30"
	case fps > 24.9 && fps < 25.1:
		return "25"
	case fps > 23.9 && fps < 24.1:
		if den == 1001 {
			return "23.976"
		}
		return "24"
	default:
		return fmt.Sprintf("%.2f", fps)
	}
}

// fallbackFormats is used when ffmpeg -list_formats fails.
var fallbackFormats = []FormatEntry{
	{"ntsc", "720x486 29.97fps interlaced"},
	{"pal", "720x576 25fps interlaced"},
	{"23ps", "1920x1080 23.976fps"},
	{"24ps", "1920x1080 24fps"},
	{"Hp25", "1920x1080 25fps"},
	{"Hp29", "1920x1080 29.97fps"},
	{"Hp30", "1920x1080 30fps"},
	{"Hp50", "1920x1080 50fps"},
	{"Hp59", "1920x1080 59.94fps"},
	{"Hp60", "1920x1080 60fps"},
	{"Hi50", "1920x1080 25fps interlaced"},
	{"Hi59", "1920x1080 29.97fps interlaced"},
	{"Hi60", "1920x1080 30fps interlaced"},
	{"hp50", "1280x720 50fps"},
	{"hp59", "1280x720 59.94fps"},
	{"hp60", "1280x720 60fps"},
}

// ProbeInput probes a specific video input (hdmi/sdi) for signal lock.
// Returns true if the device locks on the given input (not color bars).
// The caller is responsible for suspending any competing device access.
func ProbeInput(ctx context.Context, ffmpegPath, device, videoInput string) (locked bool, err error) {
	probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	args := []string{"-hide_banner", "-f", "decklink", "-video_input", videoInput, "-i", device, "-t", "1", "-f", "null", "-"}

	cmd := exec.CommandContext(probeCtx, ffmpegPath, args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err = cmd.Run()

	if err != nil {
		return false, nil // probe failed — no signal on this input
	}

	if strings.Contains(out.String(), "No input signal detected") {
		return false, nil // color bars — not a live source
	}

	return true, nil
}

// Run probes each format code and reports progress via send. Stops on ctx cancel.
func Run(ctx context.Context, ffmpegPath, device, videoInput string, send func(tui.ScanProgressMsg)) []tui.ScanResultEntry {
	formats := QueryFormats(ffmpegPath, device)
	total := len(formats)
	results := make([]tui.ScanResultEntry, 0, total)

	for i, f := range formats {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		entry := probeFormat(ctx, ffmpegPath, device, videoInput, f)
		results = append(results, entry)
		send(tui.ScanProgressMsg{
			Done:    i + 1,
			Total:   total,
			Current: fmt.Sprintf("%s (%s)", f.Code, f.Desc),
			Entry:   entry,
		})
	}
	return results
}

func probeFormat(ctx context.Context, ffmpegPath, device, videoInput string, f FormatEntry) tui.ScanResultEntry {
	probCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	args := []string{"-hide_banner", "-f", "decklink"}
	if videoInput != "" && videoInput != "auto" {
		args = append(args, "-video_input", videoInput)
	}
	args = append(args, "-format_code", f.Code, "-i", device, "-t", "1", "-f", "null", "-")

	cmd := exec.CommandContext(probCtx, ffmpegPath, args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()

	output := out.String()
	colorBars := err == nil && strings.Contains(output, "No input signal detected")

	entry := tui.ScanResultEntry{
		FormatCode:  f.Code,
		Description: f.Desc,
		Locked:      err == nil && !colorBars,
		ColorBars:   colorBars,
	}
	if err != nil {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(lines[i]); l != "" {
				entry.Err = l
				break
			}
		}
	}
	return entry
}
