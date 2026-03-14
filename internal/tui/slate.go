package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type Slate struct {
	Show  string
	Scene string
	Take  string
}

type SlateSavedMsg struct {
	Slate Slate
}

type SlateOverlay struct {
	inputs []textinput.Model
	focus  int
	out    chan Slate
}

func NewSlateOverlay(current Slate, out chan Slate) *SlateOverlay {
	show := textinput.New()
	show.Placeholder = "Show"
	show.SetValue(current.Show)
	show.Focus()

	scene := textinput.New()
	scene.Placeholder = "Scene"
	scene.SetValue(current.Scene)

	take := textinput.New()
	take.Placeholder = "Take"
	take.SetValue(current.Take)

	return &SlateOverlay{
		inputs: []textinput.Model{show, scene, take},
		out:    out,
	}
}

func (s *SlateOverlay) Init() tea.Cmd {
	return textinput.Blink
}

func (s *SlateOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, nil
		case "tab", "shift+tab":
			s.moveFocus(msg.String() == "shift+tab")
			return s, nil
		case "enter":
			slate := Slate{
				Show:  strings.TrimSpace(s.inputs[0].Value()),
				Scene: strings.TrimSpace(s.inputs[1].Value()),
				Take:  strings.TrimSpace(s.inputs[2].Value()),
			}
			if s.out != nil {
				select {
				case s.out <- slate:
				default:
				}
			}
			return nil, func() tea.Msg {
				return SlateSavedMsg{Slate: slate}
			}
		}
	}

	var cmds []tea.Cmd
	for i := range s.inputs {
		var cmd tea.Cmd
		s.inputs[i], cmd = s.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return s, tea.Batch(cmds...)
}

func (s *SlateOverlay) View() string {
	lines := []string{
		TitleBar("CLIP NAME"),
		"",
		"Show:  " + s.inputs[0].View(),
		"Scene: " + s.inputs[1].View(),
		"Take:  " + s.inputs[2].View(),
		"",
		styleDim.Render("Tab to move, Enter to save, Esc to close"),
	}
	return PanelStyle(36).Render(strings.Join(lines, "\n"))
}

func (s *SlateOverlay) Size() (int, int) {
	return 36, 9
}

func (s *SlateOverlay) moveFocus(reverse bool) {
	s.inputs[s.focus].Blur()
	if reverse {
		s.focus--
		if s.focus < 0 {
			s.focus = len(s.inputs) - 1
		}
	} else {
		s.focus = (s.focus + 1) % len(s.inputs)
	}
	s.inputs[s.focus].Focus()
}
