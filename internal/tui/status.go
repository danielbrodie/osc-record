package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// StatusPanel renders the recording status bar.
type StatusPanel struct {
	State     RecordingState
	File      string
	StartTime time.Time
	Elapsed   time.Duration
	FileSize  int64

	Device     string
	CaptureMode string
	FormatCode string

	DiskFree  uint64
	DiskTotal uint64
	DiskPath  string

	BitrateBps float64 // estimated from filesize/elapsed
}

func NewStatusPanel() StatusPanel {
	return StatusPanel{State: StateIdle}
}

func (p *StatusPanel) SetRecording(file string, t time.Time) {
	p.State = StateRecording
	p.File = file
	p.StartTime = t
	p.Elapsed = 0
	p.FileSize = 0
}

func (p *StatusPanel) SetIdle() {
	p.State = StateIdle
}

func (p *StatusPanel) SetDisk(msg DiskStatMsg) {
	p.DiskFree = msg.FreeBytes
	p.DiskTotal = msg.TotalBytes
	p.DiskPath = msg.Path
}

// View renders the status panel. blink toggles the recording indicator.
func (p StatusPanel) View(width, height int, blink bool) string {
	// Line 1: state + file + elapsed + size
	stateStr := p.renderState(blink)

	var fileInfo string
	if p.File != "" {
		fileInfo = "   " + styleText.Render(p.File) +
			"   " + styleText.Render(fmtDuration(p.Elapsed)) +
			"   " + styleText.Render(fmtBytes(p.FileSize))
	}

	line1 := stateStr + fileInfo

	// Line 2: device + disk
	deviceInfo := styleDim.Render(p.Device)
	if p.CaptureMode != "" {
		deviceInfo += styleDim.Render("  " + p.CaptureMode)
	}
	if p.FormatCode != "" {
		deviceInfo += styleDim.Render("  " + p.FormatCode)
	}

	var diskInfo string
	if p.DiskFree > 0 {
		remaining := p.estimateRemaining()
		diskInfo = styleText.Render(p.DiskPath) + "   " +
			styleText.Render(fmtBytesHuman(p.DiskFree)+" free") +
			styleDim.Render("  "+remaining)
	}

	separator := "   "
	if deviceInfo != "" && diskInfo != "" {
		separator = "   "
	}
	line2 := deviceInfo + separator + diskInfo

	content := lipgloss.JoinVertical(lipgloss.Left,
		line1,
		line2,
	)
	return PanelStyle(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, TitleBar("STATUS"), content),
	)
}

func (p StatusPanel) renderState(blink bool) string {
	switch p.State {
	case StateRecording:
		dot := "●"
		if !blink {
			dot = "○"
		}
		return styleRecording.Render(dot + " RECORDING")
	case StateStarting:
		return styleWarning.Render("◌ STARTING")
	case StateStopping:
		return styleWarning.Render("◌ STOPPING")
	case StateError:
		return styleError.Render("✗ ERROR")
	default:
		return styleIdle.Render("○ IDLE")
	}
}

func (p StatusPanel) estimateRemaining() string {
	if p.DiskFree == 0 {
		return ""
	}
	if p.BitrateBps <= 0 || p.State != StateRecording {
		return "~" + fmtBytesHuman(p.DiskFree) + " free"
	}
	secondsRemaining := float64(p.DiskFree) / p.BitrateBps
	hours := int(secondsRemaining / 3600)
	if hours > 999 {
		hours = 999
	}
	return fmt.Sprintf("~%dh remaining at current rate", hours)
}

func fmtBytes(b int64) string {
	if b == 0 {
		return "—"
	}
	return fmtBytesHuman(uint64(b))
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
