// Package scanner probes all supported decklink format codes and reports which have signal.
package scanner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

// FormatEntry is a known decklink format code.
type FormatEntry struct {
	Code string
	Desc string
}

// KnownFormats is the list of format codes to probe.
var KnownFormats = []FormatEntry{
	{"ntsc", "720x486 29.97fps interlaced"},
	{"pal", "720x576 25fps interlaced"},
	{"23ps", "1080p 23.976fps"},
	{"24ps", "1080p 24fps"},
	{"Hp25", "1080p 25fps"},
	{"Hp29", "1080p 29.97fps"},
	{"Hp30", "1080p 30fps"},
	{"Hp50", "1080p 50fps"},
	{"Hp59", "1080p 59.94fps"},
	{"Hp60", "1080p 60fps"},
	{"Hi50", "1080i 50fps"},
	{"Hi59", "1080i 59.94fps"},
	{"Hi60", "1080i 60fps"},
	{"hp50", "720p 50fps"},
	{"hp59", "720p 59.94fps"},
	{"hp60", "720p 60fps"},
}

// Run probes each format code and reports progress via send. Stops on ctx cancel.
func Run(ctx context.Context, ffmpegPath, device, videoInput string, send func(tui.ScanProgressMsg)) []tui.ScanResultEntry {
	total := len(KnownFormats)
	results := make([]tui.ScanResultEntry, 0, total)

	for i, f := range KnownFormats {
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

	entry := tui.ScanResultEntry{
		FormatCode:  f.Code,
		Description: f.Desc,
		Locked:      err == nil,
	}
	if err != nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(lines[i]); l != "" {
				entry.Err = l
				break
			}
		}
	}
	return entry
}
