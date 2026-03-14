package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard shortcuts for the TUI.
type KeyMap struct {
	Record    key.Binding
	Stop      key.Binding
	Preview   key.Binding
	ClipView  key.Binding
	SlateName key.Binding
	TakeReset key.Binding
	Confidence key.Binding
	Wizard    key.Binding
	Scanner   key.Binding
	Checklist key.Binding
	Tab       key.Binding
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Escape    key.Binding
	Help      key.Binding
	Quit      key.Binding
}

// DefaultKeyMap returns the standard key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Record: key.NewBinding(
			key.WithKeys("r", "R"),
			key.WithHelp("R", "Record"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s", "S"),
			key.WithHelp("S", "Stop"),
		),
		Preview: key.NewBinding(
			key.WithKeys("p", "P"),
			key.WithHelp("P", "Preview"),
		),
		ClipView: key.NewBinding(
			key.WithKeys("v", "V"),
			key.WithHelp("V", "View clip"),
		),
		SlateName: key.NewBinding(
			key.WithKeys("n", "N"),
			key.WithHelp("N", "Name clip"),
		),
		TakeReset: key.NewBinding(
			key.WithKeys("t", "T"),
			key.WithHelp("T", "Reset take"),
		),
		Confidence: key.NewBinding(
			key.WithKeys("c", "C"),
			key.WithHelp("C", "Confidence"),
		),
		Wizard: key.NewBinding(
			key.WithKeys("w", "W"),
			key.WithHelp("W", "Setup"),
		),
		Scanner: key.NewBinding(
			key.WithKeys("f1"),
			key.WithHelp("F1", "Signal scan"),
		),
		Checklist: key.NewBinding(
			key.WithKeys("f2"),
			key.WithHelp("F2", "Checklist"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("Tab", "Focus"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "Help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "Q", "ctrl+c"),
			key.WithHelp("Q", "Quit"),
		),
	}
}
