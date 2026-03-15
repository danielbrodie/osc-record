package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SignalPanel renders the device signal status, audio meters, and timecode.
type SignalPanel struct {
	// Signal state
	locked     bool
	colorBars  bool
	probing    bool
	input      string
	format     string
	resolution string
	fps        string
	signalErr  string

	// Audio
	audioLeft  float64
	audioRight float64

	// Timecode
	tc string
}

func NewSignalPanel() SignalPanel {
	return SignalPanel{audioLeft: -60, audioRight: -60}
}

func (p *SignalPanel) Init() tea.Cmd { return nil }

func (p *SignalPanel) Update(msg SignalStateMsg) {
	p.locked = msg.Locked
	p.colorBars = msg.ColorBars
	p.probing = msg.Probing
	p.input = msg.Input
	p.format = msg.Format
	p.resolution = msg.Resolution
	p.fps = msg.FPS
	p.signalErr = msg.Err
}

func (p *SignalPanel) UpdateAudio(msg AudioLevelMsg) {
	p.audioLeft = msg.Left
	p.audioRight = msg.Right
}

func (p *SignalPanel) UpdateTC(msg TimecodeMsg) {
	p.tc = msg.TC
}

// View renders the signal panel at the given width and height.
func (p SignalPanel) View(width, height int) string {
	inner := width - 4 // border + padding

	// SDI row
	sdiStatus, sdiDetail := p.inputRow("SDI")
	// HDMI row
	hdmiStatus, hdmiDetail := p.inputRow("HDMI")

	sdiLine := sdiStatus + " " + styleText.Render(sdiDetail)
	hdmiLine := hdmiStatus + " " + styleText.Render(hdmiDetail)

	// Audio meters
	meterW := inner - 14 // "L  " prefix + " -XX dBFS" suffix
	if meterW < 4 {
		meterW = 4
	}
	leftBar := VUBar(p.audioLeft, meterW)
	rightBar := VUBar(p.audioRight, meterW)
	leftLevel := fmtLevel(p.audioLeft)
	rightLevel := fmtLevel(p.audioRight)

	leftLine := styleIdle.Render("L  ") + leftBar + styleDim.Render("  "+leftLevel)
	rightLine := styleIdle.Render("R  ") + rightBar + styleDim.Render("  "+rightLevel)

	// Timecode
	var tcLine string
	if p.tc != "" {
		tcLine = styleIdle.Render("TC ") + styleTC.Render(p.tc)
	}

	lines := []string{
		sdiLine,
		hdmiLine,
		"",
		leftLine,
		rightLine,
	}
	if tcLine != "" {
		lines = append(lines, "", tcLine)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	title := TitleBar("SIGNAL")
	body := PanelStyle(width).Render(lipgloss.JoinVertical(lipgloss.Left, title, content))
	return body
}

func (p SignalPanel) inputRow(inputName string) (indicator, detail string) {
	if p.probing {
		indicator = styleWarning.Render("⟳")
		detail = inputName + "  probing..."
		return
	}
	if inputName == p.input || (p.input == "" && inputName == "SDI") {
		if p.colorBars {
			indicator = styleWarning.Render("◑")
			detail = fmt.Sprintf("%-4s  color bars (no source)", inputName)
		} else if p.locked {
			indicator = styleLocked.Render("●")
			detail = fmt.Sprintf("%-4s  %s  %sfps  %s", inputName, p.resolution, p.fps, p.format)
		} else if p.signalErr != "" {
			indicator = styleError.Render("○")
			detail = inputName + "  " + p.signalErr
		} else {
			indicator = styleError.Render("○")
			detail = inputName + "  no signal"
		}
	} else {
		indicator = styleIdle.Render("○")
		detail = inputName + "  —"
	}
	return
}

func fmtLevel(db float64) string {
	if db <= -60 {
		return "-inf"
	}
	return fmt.Sprintf("%+.0f dBFS", db)
}
