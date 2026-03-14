package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ScanResultEntry holds the result for a single format code.
type ScanResultEntry struct {
	FormatCode  string
	Description string
	Locked      bool
	Err         string
}

// ScanCompleteMsg is sent when the scanner finishes all probes.
type ScanCompleteMsg struct {
	Results []ScanResultEntry
}

// ScanProgressMsg is sent after each individual probe.
type ScanProgressMsg struct {
	Done    int
	Total   int
	Current string
	Entry   ScanResultEntry
}

// ScannerOverlay shows the live format scanner.
type ScannerOverlay struct {
	results  []ScanResultEntry
	done     int
	total    int
	current  string
	finished bool
	width    int
	height   int
}

func NewScannerOverlay(width, height int) ScannerOverlay {
	return ScannerOverlay{width: width, height: height}
}

func (s ScannerOverlay) Init() tea.Cmd { return nil }

func (s ScannerOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case ScanProgressMsg:
		s.done = msg.Done
		s.total = msg.Total
		s.current = msg.Current
		s.results = append(s.results, msg.Entry)
		return s, nil
	case ScanCompleteMsg:
		s.results = msg.Results
		s.finished = true
		return s, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "Q":
			return nil, func() tea.Msg { return ScanCancelledMsg{} }
		}
	}
	return s, nil
}

// ScanCancelledMsg is emitted when the user closes the scanner.
type ScanCancelledMsg struct{}

func (s ScannerOverlay) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("SIGNAL SCANNER") + "\n\n")

	if s.total > 0 {
		pct := float64(s.done) / float64(s.total)
		bar := progressBar(pct, 40)
		b.WriteString(fmt.Sprintf("  %s  %d/%d\n", bar, s.done, s.total))
		if s.current != "" && !s.finished {
			b.WriteString(styleDim.Render("  Probing: "+s.current) + "\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString(styleDim.Render("  Scanning...") + "\n\n")
	}

	if len(s.results) > 0 {
		b.WriteString(styleText.Render("  FORMAT           RESULT") + "\n")
		b.WriteString(styleDim.Render("  " + strings.Repeat("─", 36)) + "\n")
		for _, r := range s.results {
			icon := styleDim.Render("  ○ ")
			status := styleDim.Render("no signal")
			if r.Locked {
				icon = styleLocked.Render("  ● ")
				status = styleLocked.Render("LOCKED")
			} else if r.Err != "" {
				icon = styleError.Render("  ✗ ")
				status = styleError.Render("error")
			}
			line := fmt.Sprintf("%-10s  %s", r.FormatCode, status)
			b.WriteString(icon + styleText.Render(line) + "\n")
		}
	}

	if s.finished {
		locked := 0
		for _, r := range s.results {
			if r.Locked {
				locked++
			}
		}
		b.WriteString("\n" + styleLocked.Render(fmt.Sprintf("  Scan complete — %d format(s) with signal", locked)) + "\n")
		b.WriteString(styleDim.Render("  [Esc] close"))
	} else {
		b.WriteString("\n" + styleDim.Render("  [Esc] cancel"))
	}

	return PanelStyle(62).Render(b.String())
}

func (s ScannerOverlay) Size() (int, int) { return 66, 30 }

// progressBar renders a simple ASCII progress bar.
func progressBar(pct float64, width int) string {
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return styleLocked.Render(bar[:filled]) + styleDim.Render(bar[filled:])
}
