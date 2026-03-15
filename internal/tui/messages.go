package tui

import "time"

// SignalStateMsg is sent by the signal poller when it completes a probe.
type SignalStateMsg struct {
	Device     string
	Input      string // "sdi" | "hdmi" | ""
	Format     string // format code, e.g. "Hp59"
	Resolution string // e.g. "1920x1080"
	FPS        string // e.g. "59.94"
	Locked     bool
	ColorBars  bool   // true when card locked but no live source (outputting color bars)
	Err        string // non-empty if probe errored
}

// AudioLevelMsg carries left/right audio levels in dBFS (-inf to 0).
type AudioLevelMsg struct {
	Left  float64
	Right float64
}

// TimecodeMsg carries an embedded LTC/VITC timecode string.
type TimecodeMsg struct {
	TC string // "HH:MM:SS:FF"
}

// OSCReceivedMsg is sent when an OSC packet arrives on the listener.
type OSCReceivedMsg struct {
	Address string
	Args    string // space-separated rendered args
	Source  string // source IP:port
	Time    time.Time
}

// RecordingStartedMsg is sent when ffmpeg successfully opens the device and starts writing.
type RecordingStartedMsg struct {
	File   string
	Device string
	Time   time.Time
}

// RecordingStoppedMsg is sent when recording ends cleanly.
type RecordingStoppedMsg struct {
	File      string
	Device    string
	Duration  time.Duration
	SizeBytes int64
}

// RecordingCrashedMsg is sent when the recording ffmpeg process exits unexpectedly.
type RecordingCrashedMsg struct {
	File        string
	Device      string
	Err         error
	Recoverable bool
}

// RecordingResumedMsg is sent after a successful crash recovery.
type RecordingResumedMsg struct {
	Device string
}

// FileSizeMsg is sent periodically with the current output file size.
type FileSizeMsg struct {
	File      string
	SizeBytes int64
}

// ClipVerifiedMsg is sent after ffprobe validates a completed clip.
type ClipVerifiedMsg struct {
	File   string
	OK     bool
	Errors []string
}

// DiskStatMsg is sent periodically with disk usage for the output directory.
type DiskStatMsg struct {
	Path      string
	FreeBytes uint64
	TotalBytes uint64
}

// CheckResult holds the outcome of a single pre-show checklist item.
type CheckResult struct {
	Name    string
	OK      bool
	Detail  string // shown next to the result
	Fix     string // hint shown when not OK
}

// ChecklistResultMsg is sent when the pre-show checklist completes.
type ChecklistResultMsg struct {
	Results []CheckResult
}

// ScanResultMsg is sent for each format tested by the signal scanner.
type ScanResultMsg struct {
	Input   string // "sdi" | "hdmi"
	Format  string // format code
	Desc    string // human description
	Locked  bool
	Preview []byte // JPEG thumbnail, nil if not captured
}

// ScanDoneMsg is sent when the signal scanner has tested all formats.
type ScanDoneMsg struct {
	LockedCount int
}

// TermSizeMsg is sent when the terminal is resized.
type TermSizeMsg struct {
	Width  int
	Height int
}

// TickMsg is sent at regular intervals for blinking indicators and elapsed time updates.
type TickMsg struct {
	Time time.Time
}

// LogMsg appends a message to the event log panel.
type LogMsg struct {
	Time time.Time
	Text string
}

// ErrorBannerMsg shows a persistent amber error banner until dismissed.
type ErrorBannerMsg struct {
	Text string
}

// ClearBannerMsg dismisses the error banner.
type ClearBannerMsg struct{}

// PreviewGrabbedMsg is sent after a frame preview is captured and opened.
type PreviewGrabbedMsg struct {
	Path string
	Err  error
}

// AutoDetectProgressMsg reports auto-detection progress to the TUI.
type AutoDetectProgressMsg struct {
	Phase  string // "input-probe", "format-scan", "complete", "failed"
	Detail string // human-readable status, e.g. "Probing HDMI..."
}

// AutoDetectCompleteMsg is sent when auto-detection finishes.
type AutoDetectCompleteMsg struct {
	VideoInput string // "hdmi" or "sdi"; empty if BothLocked
	FormatCode string // e.g. "Hp59"; empty if not yet resolved
	FormatDesc string // e.g. "1080p 59.94fps"
	BothLocked bool   // true if both inputs had signal — caller must disambiguate
	Err        error
}

// InputChosenMsg is sent when the user picks an input from the disambiguation overlay.
type InputChosenMsg struct {
	VideoInput string // "hdmi" or "sdi"
}

// ConfigUpdatedMsg notifies the TUI that config values changed after auto-detect.
type ConfigUpdatedMsg struct {
	VideoInput string
	FormatCode string
}
