package tui

import "github.com/charmbracelet/lipgloss"

// Palette — all colors defined once here.
const (
	colorBg         = "#0d0d0d"
	colorBorder     = "#2a2a2a"
	colorTitle      = "#f5c400"
	colorGreen      = "#00e676"
	colorAmber      = "#ffab40"
	colorRed        = "#ff5252"
	colorIdle       = "#78909c"
	colorOSCAddr    = "#80d8ff"
	colorTimecode   = "#e040fb"
	colorKeyHint    = "#546e7a"
	colorDimText    = "#4a4a4a"
	colorText       = "#e0e0e0"
)

var (
	// Base styles
	styleBg = lipgloss.NewStyle().Background(lipgloss.Color(colorBg))

	// Panel border style — rounded corners, dim border
	panelBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Background(lipgloss.Color(colorBg)).
		Padding(0, 1)

	// Panel title — amber, bold
	styleTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTitle)).
		Bold(true)

	// Status indicators
	styleLocked  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	styleIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorIdle))
	styleText    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDimText))

	// OSC monitor
	styleOSCAddr    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOSCAddr))
	styleOSCRecord  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	styleOSCStop    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	styleOSCUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDimText))

	// Timecode
	styleTC = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTimecode))

	// Key hints bar
	styleKey     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTitle)).Bold(true)
	styleKeyHint = lipgloss.NewStyle().Foreground(lipgloss.Color(colorKeyHint))

	// Error banner
	styleBanner = lipgloss.NewStyle().
		Background(lipgloss.Color(colorAmber)).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Padding(0, 2)

	// Recording state
	styleRecording = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorGreen)).
		Bold(true)
)

// PanelStyle returns a lipgloss style for a panel with the given inner width.
func PanelStyle(width int) lipgloss.Style {
	return panelBorder.Width(width)
}

// TitleBar renders a panel title string.
func TitleBar(label string) string {
	return styleTitle.Render(label)
}

// VUBar renders a horizontal VU meter bar.
// level is in dBFS (0 = clip, -60 = silence). width is total chars.
func VUBar(level float64, width int) string {
	if width <= 0 {
		return ""
	}
	// Map -60..0 dBFS → 0..width fill
	const floor = -60.0
	normalized := (level - floor) / (0 - floor)
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	filled := int(normalized * float64(width))
	empty := width - filled

	// Color: green below -18, amber -18 to -6, red above -6
	var color string
	switch {
	case level > -6:
		color = colorRed
	case level > -18:
		color = colorAmber
	default:
		color = colorGreen
	}

	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(
		repeat("█", filled),
	) + styleDim.Render(repeat("░", empty))
	return bar
}

func repeat(s string, n int) string {
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}

// KeyHints renders the bottom key hint bar.
func KeyHints(pairs ...string) string {
	// pairs: alternating key, description
	parts := make([]string, 0, len(pairs))
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleKey.Render("[" + pairs[i] + "]")
		d := styleKeyHint.Render(" " + pairs[i+1])
		parts = append(parts, k+d)
	}
	sep := styleKeyHint.Render("  ")
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
