package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ClipsPanel renders the session clip list.
type ClipsPanel struct {
	clips    []ClipInfo
	selected int
}

func NewClipsPanel() ClipsPanel {
	return ClipsPanel{}
}

func (p *ClipsPanel) SetClips(clips []ClipInfo) {
	p.clips = clips
	if p.selected >= len(clips) && len(clips) > 0 {
		p.selected = len(clips) - 1
	}
}

// View renders the clips panel.
func (p ClipsPanel) View(width, height int) string {
	innerH := height - 3 // title + border
	lines := make([]string, 0, len(p.clips))

	for _, c := range p.clips {
		taggedName := c.File
		if c.Device != "" {
			taggedName = "[" + c.Device + "] " + c.File
		}
		name := shortName(taggedName, width-24)
		dur := fmtDuration(c.Duration)
		size := fmtBytes(c.SizeBytes)
		status := p.verifyStatus(c)
		line := fmt.Sprintf("#%-2d  %-*s  %5s  %6s  %s",
			c.Index, width-28, name, dur, size, status)
		lines = append(lines, styleText.Render(line))
	}

	if len(lines) == 0 {
		lines = append(lines, styleDim.Render("  No clips recorded yet"))
	}

	// Trim to available height
	if len(lines) > innerH {
		lines = lines[len(lines)-innerH:]
	}

	content := strings.Join(lines, "\n")
	return PanelStyle(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, TitleBar("CLIPS"), content),
	)
}

func (p ClipsPanel) verifyStatus(c ClipInfo) string {
	if c.Verified == nil {
		return styleWarning.Render("…")
	}
	if *c.Verified {
		return styleLocked.Render("✓")
	}
	return styleError.Render("✗")
}

func shortName(path string, maxLen int) string {
	// Extract basename
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if len(name) <= maxLen {
		return name
	}
	if maxLen <= 3 {
		return "..."
	}
	return name[:maxLen-3] + "..."
}
