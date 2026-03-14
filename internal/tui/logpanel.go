package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// LogPanel is the scrollable event log at the bottom.
type LogPanel struct {
	entries []LogMsg
	vp      viewport.Model
	ready   bool
	width   int
	height  int
}

func NewLogPanel() LogPanel {
	return LogPanel{}
}

func (p LogPanel) Update(msg tea.Msg) (LogPanel, tea.Cmd) {
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	return p, cmd
}

func (p *LogPanel) Append(msg LogMsg) {
	p.entries = append(p.entries, msg)
	p.rebuildContent()
	p.vp.GotoBottom()
}

func (p *LogPanel) rebuildContent() {
	lines := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		ts := e.Time.Format("15:04:05")
		line := styleDim.Render(ts+"  ") + styleText.Render(e.Text)
		lines = append(lines, line)
	}
	p.vp.SetContent(strings.Join(lines, "\n"))
}

// View renders the log panel.
func (p *LogPanel) View(width, height int) string {
	innerW := width - 4
	innerH := height - 3

	if !p.ready || p.width != width || p.height != height {
		p.vp = viewport.New(innerW, innerH)
		p.vp.Style = lipgloss.NewStyle()
		p.width = width
		p.height = height
		p.ready = true
		p.rebuildContent()
		p.vp.GotoBottom()
	}

	placeholder := ""
	if len(p.entries) == 0 {
		placeholder = styleDim.Render(fmt.Sprintf("  Waiting for events..."))
	}

	var content string
	if placeholder != "" {
		content = placeholder
	} else {
		content = p.vp.View()
	}

	return PanelStyle(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, TitleBar("LOG"), content),
	)
}
