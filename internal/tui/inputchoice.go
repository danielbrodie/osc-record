package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// InputChoiceOverlay prompts the user to choose between HDMI and SDI
// when both inputs have a live signal during auto-detection.
type InputChoiceOverlay struct {
	selected int
	options  []inputOption
}

type inputOption struct {
	label string
	value string // "hdmi" or "sdi"
}

// NewInputChoiceOverlay creates the disambiguation overlay.
func NewInputChoiceOverlay() InputChoiceOverlay {
	return InputChoiceOverlay{
		options: []inputOption{
			{label: "HDMI", value: "hdmi"},
			{label: "SDI", value: "sdi"},
		},
	}
}

func (o InputChoiceOverlay) Init() tea.Cmd { return nil }

func (o InputChoiceOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if o.selected > 0 {
				o.selected--
			}
		case "down", "j":
			if o.selected < len(o.options)-1 {
				o.selected++
			}
		case "enter":
			chosen := o.options[o.selected].value
			return nil, func() tea.Msg { return InputChosenMsg{VideoInput: chosen} }
		case "esc":
			// Send empty string to unblock the auto-detect goroutine.
			return nil, func() tea.Msg { return InputChosenMsg{VideoInput: ""} }
		}
	}
	return o, nil
}

func (o InputChoiceOverlay) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("INPUT SELECTION") + "\n\n")
	b.WriteString(styleText.Render("Both HDMI and SDI have a live signal.") + "\n")
	b.WriteString(styleText.Render("Which input is your camera connected to?") + "\n\n")

	for i, opt := range o.options {
		prefix := "  "
		if i == o.selected {
			prefix = styleWarning.Render("▶ ")
		}
		b.WriteString(fmt.Sprintf("%s%s\n", prefix, styleText.Render(opt.label)))
	}

	b.WriteString("\n" + styleDim.Render("[↑↓] select  [Enter] confirm  [Esc] skip"))

	return PanelStyle(50).Render(b.String())
}

func (o InputChoiceOverlay) Size() (int, int) { return 54, 12 }
