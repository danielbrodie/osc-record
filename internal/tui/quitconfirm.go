package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// QuitConfirmOverlay asks for confirmation before quitting while recording.
type QuitConfirmOverlay struct {
	filename string
}

// QuitConfirmedMsg is sent when the user confirms quit while recording.
type QuitConfirmedMsg struct{}

// QuitCancelledMsg is sent when the user cancels quit.
type QuitCancelledMsg struct{}

func NewQuitConfirm(filename string) QuitConfirmOverlay {
	return QuitConfirmOverlay{filename: filename}
}

func (q QuitConfirmOverlay) Init() tea.Cmd { return nil }

func (q QuitConfirmOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "Y", "enter":
			return nil, func() tea.Msg { return QuitConfirmedMsg{} }
		case "n", "N", "esc":
			return nil, func() tea.Msg { return QuitCancelledMsg{} }
		}
	}
	return q, nil
}

func (q QuitConfirmOverlay) View() string {
	var b strings.Builder
	b.WriteString(styleError.Render("⚠  RECORDING IN PROGRESS") + "\n\n")
	b.WriteString(styleText.Render("  File: ") + styleLocked.Render(q.filename) + "\n\n")
	b.WriteString(styleText.Render("  Quitting now will stop the recording and\n"))
	b.WriteString(styleText.Render("  save whatever has been captured so far.\n\n"))
	b.WriteString(styleWarning.Render("  Stop recording and quit?") + "\n\n")
	b.WriteString(styleLocked.Render("  [Y] Yes, stop and quit") + "   " + styleDim.Render("[N / Esc] Cancel"))
	return PanelStyle(52).Render(b.String())
}

func (q QuitConfirmOverlay) Size() (int, int) { return 56, 14 }
