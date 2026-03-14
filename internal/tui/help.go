package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// HelpOverlay renders a keyboard reference overlay.
type HelpOverlay struct{}

// HelpDismissMsg is sent when the user closes the help overlay.
type HelpDismissMsg struct{}

func NewHelpOverlay() HelpOverlay { return HelpOverlay{} }

func (h HelpOverlay) Init() tea.Cmd { return nil }

func (h HelpOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc", "q", "Q", "?":
			return nil, func() tea.Msg { return HelpDismissMsg{} }
		}
	}
	return h, nil
}

func (h HelpOverlay) View() string {
	rows := [][2]string{
		{"R", "Start recording (manual override)"},
		{"S", "Stop recording (manual override)"},
		{"N", "Set show / scene / take (slate)"},
		{"T", "Reset take counter to 1"},
		{"V", "Open last clip in system viewer"},
		{"P", "Grab preview frame → system viewer"},
		{"F1", "Signal scanner — probe all format codes"},
		{"F2", "Pre-show checklist"},
		{"W", "Setup wizard (device, OSC, output)"},
		{"?", "This help screen"},
		{"Esc", "Dismiss overlay / clear banner"},
		{"Q / Ctrl+C", "Quit (safe — stops recording first)"},
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render("KEYBOARD REFERENCE") + "\n\n")

	for _, row := range rows {
		key := styleLocked.Render("  " + padRight(row[0], 12))
		desc := styleText.Render(row[1])
		b.WriteString(key + desc + "\n")
	}

	b.WriteString("\n")
	b.WriteString(styleText.Render("  OSC triggers") + "\n")
	b.WriteString(styleDim.Render("  Record/stop addresses are configured in config.toml") + "\n")
	b.WriteString(styleDim.Render("  or via the setup wizard [W].") + "\n")
	b.WriteString("\n" + styleDim.Render("  [Esc] or [?] close"))

	return PanelStyle(60).Render(b.String())
}

func (h HelpOverlay) Size() (int, int) { return 64, 24 }

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
