package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// OSCEntry is a single received OSC message.
type OSCEntry struct {
	Time    time.Time
	Address string
	Args    string
	Source  string
}

// OSCPanel is the OSC monitor panel.
type OSCPanel struct {
	entries    []OSCEntry
	vp         viewport.Model
	recordAddr string
	stopAddr   string
	autoScroll bool
	ready      bool
	width      int
	height     int
}

func NewOSCPanel(recordAddr, stopAddr string) OSCPanel {
	return OSCPanel{
		recordAddr: recordAddr,
		stopAddr:   stopAddr,
		autoScroll: true,
	}
}

func (p OSCPanel) Init() tea.Cmd { return nil }

func (p OSCPanel) Update(msg tea.Msg) (OSCPanel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case OSCReceivedMsg:
		p.entries = append(p.entries, OSCEntry{
			Time:    msg.Time,
			Address: msg.Address,
			Args:    msg.Args,
			Source:  msg.Source,
		})
		p.rebuildContent()
		if p.autoScroll {
			p.vp.GotoBottom()
		}
	default:
		p.vp, cmd = p.vp.Update(msg)
	}
	return p, cmd
}

func (p *OSCPanel) Append(msg OSCReceivedMsg) {
	p.entries = append(p.entries, OSCEntry{
		Time:    msg.Time,
		Address: msg.Address,
		Args:    msg.Args,
		Source:  msg.Source,
	})
	p.rebuildContent()
	if p.autoScroll {
		p.vp.GotoBottom()
	}
}

func (p *OSCPanel) rebuildContent() {
	lines := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		ts := e.Time.Format("15:04:05.000")
		addr := p.renderAddr(e.Address)
		source := styleDim.Render(e.Source)
		var argPart string
		if e.Args != "" {
			argPart = styleDim.Render(" " + e.Args)
		}
		line := styleDim.Render(ts+"  ") + addr + argPart + "  " + source
		lines = append(lines, line)
	}
	p.vp.SetContent(strings.Join(lines, "\n"))
}

func (p OSCPanel) renderAddr(addr string) string {
	switch {
	case addr == p.recordAddr:
		return styleOSCRecord.Render(addr)
	case addr == p.stopAddr:
		return styleOSCStop.Render(addr)
	default:
		return styleOSCAddr.Render(addr)
	}
}

// View renders the OSC panel.
func (p *OSCPanel) View(width, height int) string {
	innerW := width - 4
	innerH := height - 3 // title + border

	if !p.ready || p.width != width || p.height != height {
		p.vp = viewport.New(innerW, innerH)
		p.vp.Style = lipgloss.NewStyle()
		p.width = width
		p.height = height
		p.ready = true
		p.rebuildContent()
		p.vp.GotoBottom()
	}

	title := TitleBar("OSC")
	placeholder := ""
	if len(p.entries) == 0 {
		placeholder = styleDim.Render(fmt.Sprintf("  Listening on port... waiting for packets"))
	}

	var content string
	if placeholder != "" {
		content = placeholder
	} else {
		content = p.vp.View()
	}

	return PanelStyle(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, content),
	)
}
