package scanner

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DShowFormat is a video format supported by a dshow capture device.
type DShowFormat struct {
	VideoSize string // e.g. "1920x1080"
	FrameRate string // e.g. "24" or "29.97"
}

var dshowMaxRe = regexp.MustCompile(`max s=(\d+x\d+)\s+fps=([\d.]+)`)
var blackDurationRe = regexp.MustCompile(`black_duration:([\d.]+)`)
var framesCapturedRe = regexp.MustCompile(`frame=\s*[1-9]`)

// QueryDShowFormats returns a de-duplicated, HD-first list of (size, fps) pairs
// supported by the device.
func QueryDShowFormats(ffmpegPath, device string) []DShowFormat {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	input := "video=" + device
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-f", "dshow", "-list_options", "true", "-i", input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()

	seen := make(map[string]bool)
	var formats []DShowFormat
	for _, line := range strings.Split(stderr.String(), "\n") {
		line = strings.TrimRight(line, "\r")
		m := dshowMaxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		size := m[1]
		fps := normalizeFPS(m[2])
		key := size + "@" + fps
		if seen[key] {
			continue
		}
		seen[key] = true
		formats = append(formats, DShowFormat{VideoSize: size, FrameRate: fps})
	}

	// Sort: larger resolution first, then lower fps first (within same resolution).
	sort.SliceStable(formats, func(i, j int) bool {
		ai := resArea(formats[i].VideoSize)
		aj := resArea(formats[j].VideoSize)
		if ai != aj {
			return ai > aj
		}
		fi, _ := strconv.ParseFloat(formats[i].FrameRate, 64)
		fj, _ := strconv.ParseFloat(formats[j].FrameRate, 64)
		return fi < fj
	})

	return formats
}

// DetectDShowSignal probes dshow formats to find the one with a live signal.
// Tries HD formats first. Returns the first format that produces non-black frames.
// send is called with a progress string before each probe; may be nil.
func DetectDShowSignal(ctx context.Context, ffmpegPath, device string, send func(string)) (*DShowFormat, error) {
	formats := QueryDShowFormats(ffmpegPath, device)
	if len(formats) == 0 {
		return nil, fmt.Errorf("could not enumerate formats for device %q — check device connection", device)
	}

	for _, f := range formats {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if send != nil {
			send(fmt.Sprintf("Probing %s @ %s fps...", f.VideoSize, f.FrameRate))
		}
		ok, err := probeDShowFormat(ctx, ffmpegPath, device, f)
		if err != nil {
			continue // format rejected by device — try next
		}
		if ok {
			return &f, nil
		}
	}

	return nil, fmt.Errorf("no signal detected on device %q — check that your source is connected and sending video", device)
}

// probeDShowFormat returns true if the format produces non-black video frames.
func probeDShowFormat(ctx context.Context, ffmpegPath, device string, f DShowFormat) (bool, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	input := "video=" + device
	cmd := exec.CommandContext(probeCtx, ffmpegPath,
		"-hide_banner",
		"-f", "dshow",
		"-video_size", f.VideoSize,
		"-framerate", f.FrameRate,
		"-i", input,
		"-t", "1",
		"-vf", "blackdetect=d=0:pix_th=0.1",
		"-an",
		"-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // dshow always exits non-zero from Go on Windows — ignore exit code

	output := stderr.String()

	// If no frames were captured the device rejected this format entirely.
	if !framesCapturedRe.MatchString(output) {
		return false, fmt.Errorf("no frames captured for %s@%s", f.VideoSize, f.FrameRate)
	}

	// blackdetect reports segments of black video. If total black duration
	// covers nearly the entire 1-second clip, there is no live signal.
	var totalBlack float64
	for _, m := range blackDurationRe.FindAllStringSubmatch(output, -1) {
		d, _ := strconv.ParseFloat(m[1], 64)
		totalBlack += d
	}
	return totalBlack < 0.9, nil
}

// normalizeFPS maps raw fps strings to standard broadcast representations.
func normalizeFPS(raw string) string {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	standards := []struct {
		val float64
		str string
	}{
		{23.976, "23.976"},
		{24.0, "24"},
		{25.0, "25"},
		{29.97, "29.97"},
		{30.0, "30"},
		{47.95, "47.95"},
		{48.0, "48"},
		{50.0, "50"},
		{59.94, "59.94"},
		{60.0, "60"},
		{119.88, "119.88"},
		{120.0, "120"},
	}
	for _, s := range standards {
		if math.Abs(f-s.val) < 0.015 {
			return s.str
		}
	}
	return fmt.Sprintf("%.3f", f)
}

// resArea returns the pixel area of a "WxH" size string.
func resArea(size string) int {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return 0
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	return w * h
}
