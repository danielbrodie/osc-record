package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielbrodie/osc-record/internal/config"
	"github.com/danielbrodie/osc-record/internal/tui"
)

type Entry struct {
	File      string
	Show      string
	Scene     string
	Take      string
	Timecode  string
	Duration  time.Duration
	SizeBytes int64
	Codec     string
	Status    string
}

type probeData struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType string            `json:"codec_type"`
	CodecName string            `json:"codec_name"`
	Tags      map[string]string `json:"tags"`
}

type probeFormat struct {
	Duration string            `json:"duration"`
	Size     string            `json:"size"`
	Tags     map[string]string `json:"tags"`
}

func Write(clips []tui.ClipInfo, cfg config.Config, outputDir string) error {
	entries := make([]Entry, 0, len(clips))
	for _, clip := range clips {
		entry, err := buildEntry(filepath.Join(outputDir, clip.File), clip)
		if err != nil {
			entry = fallbackEntry(clip, cfg)
		}
		if entry.Show == "" {
			entry.Show = cfg.Recording.Show
		}
		if entry.Scene == "" {
			entry.Scene = cfg.Recording.Scene
		}
		if entry.Take == "" {
			entry.Take = cfg.Recording.Take
		}
		entries = append(entries, entry)
	}

	content := render(entries, cfg, outputDir, time.Now())
	filename := manifestFilename(cfg.Recording.Show, time.Now())
	return os.WriteFile(filepath.Join(outputDir, filename), []byte(content), 0o644)
}

func BuildEntriesFromDir(dir string) ([]Entry, error) {
	patterns := []string{"*.mp4", "*.mov"}
	entries := make([]Entry, 0)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, match := range matches {
			entry, err := buildEntry(match, tui.ClipInfo{File: filepath.Base(match)})
			if err != nil {
				return nil, err
			}
			entry.Status = "✓"
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func RenderForDirectory(entries []Entry, cfg config.Config, outputDir string) string {
	return render(entries, cfg, outputDir, time.Now())
}

func ManifestFilename(show string, now time.Time) string {
	return manifestFilename(show, now)
}

func buildEntry(path string, clip tui.ClipInfo) (Entry, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return Entry{}, err
	}

	output, err := exec.Command(ffprobePath, "-v", "error", "-show_streams", "-show_format", "-of", "json", path).Output()
	if err != nil {
		return Entry{}, err
	}

	var probe probeData
	if err := json.Unmarshal(output, &probe); err != nil {
		return Entry{}, err
	}

	entry := Entry{
		File:      filepath.Base(path),
		Duration:  clip.Duration,
		SizeBytes: clip.SizeBytes,
		Status:    clipStatus(clip),
	}

	for _, stream := range probe.Streams {
		if stream.CodecType == "video" && entry.Codec == "" {
			entry.Codec = stream.CodecName
		}
		if entry.Timecode == "" && stream.Tags["timecode"] != "" {
			entry.Timecode = stream.Tags["timecode"]
		}
	}

	if probe.Format.Duration != "" {
		if durationSec, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			entry.Duration = time.Duration(durationSec * float64(time.Second))
		}
	}
	if probe.Format.Size != "" {
		if sizeBytes, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
			entry.SizeBytes = sizeBytes
		}
	}
	entry.Show = probe.Format.Tags["show"]
	entry.Scene = probe.Format.Tags["scene"]
	entry.Take = probe.Format.Tags["take"]

	if entry.Status == "…" {
		entry.Status = "✓"
	}
	return entry, nil
}

func fallbackEntry(clip tui.ClipInfo, cfg config.Config) Entry {
	return Entry{
		File:      clip.File,
		Show:      cfg.Recording.Show,
		Scene:     cfg.Recording.Scene,
		Take:      cfg.Recording.Take,
		Duration:  clip.Duration,
		SizeBytes: clip.SizeBytes,
		Status:    clipStatus(clip),
	}
}

func clipStatus(clip tui.ClipInfo) string {
	if clip.Verified == nil {
		return "…"
	}
	if *clip.Verified {
		return "✓"
	}
	if len(clip.VerifyErr) > 0 {
		return "✗ (" + strings.Join(clip.VerifyErr, "; ") + ")"
	}
	return "✗"
}

func render(entries []Entry, cfg config.Config, outputDir string, now time.Time) string {
	var totalDuration time.Duration
	var totalSize int64
	for _, entry := range entries {
		totalDuration += entry.Duration
		totalSize += entry.SizeBytes
	}

	lines := []string{
		"osc-record session manifest",
		"Generated: " + now.Format("2006-01-02 15:04:05"),
		"Show:      " + valueOrDash(cfg.Recording.Show),
		"Devices:   " + deviceSummary(cfg),
		"Output:    " + outputDir,
		"",
		"#\tFile\tShow\tScene\tTake\tTC In\tDuration\tSize\tStatus\tCodec",
	}

	for i, entry := range entries {
		lines = append(lines, fmt.Sprintf(
			"%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			i+1,
			entry.File,
			valueOrDash(entry.Show),
			valueOrDash(entry.Scene),
			valueOrDash(entry.Take),
			valueOrDash(entry.Timecode),
			fmtDuration(entry.Duration),
			fmtBytesHuman(uint64(maxInt64(entry.SizeBytes, 0))),
			entry.Status,
			valueOrDash(entry.Codec),
		))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Total: %d clips, %s, %s", len(entries), fmtDuration(totalDuration), fmtBytesHuman(uint64(maxInt64(totalSize, 0)))))
	return strings.Join(lines, "\n") + "\n"
}

func manifestFilename(show string, now time.Time) string {
	if strings.TrimSpace(show) == "" {
		return "session-manifest.txt"
	}
	return fmt.Sprintf("%s-%s-manifest.txt", sanitize(show), now.Format("2006-01-02"))
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "-")
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return -1
		default:
			return r
		}
	}, value)
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func deviceSummary(cfg config.Config) string {
	devices := cfg.ActiveDevices()
	parts := make([]string, 0, len(devices))
	for _, device := range devices {
		parts = append(parts, fmt.Sprintf(
			"%s (%s %s)",
			valueOrDash(device.Name),
			valueOrDash(device.CaptureMode),
			valueOrDash(device.FormatCode),
		))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func fmtBytesHuman(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.0fMB", float64(b)/mb)
	case b >= kb:
		return fmt.Sprintf("%.0fKB", float64(b)/kb)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
