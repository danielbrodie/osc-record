package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danielbrodie/osc-record/internal/devices"
)

// WizardStep is a numbered wizard step.
type WizardStep int

const (
	WizardStepDevice WizardStep = iota
	WizardStepOSCRecord
	WizardStepOSCStop
	WizardStepOutput
	WizardStepDone
)

// WizardResult is the config saved when the wizard completes.
type WizardResult struct {
	DeviceName    string
	RecordAddress string
	StopAddress   string
	OutputDir     string
	Prefix        string
}

// WizardDoneMsg is sent when the wizard completes.
type WizardDoneMsg struct {
	Result WizardResult
}

// WizardCancelledMsg is sent when the user presses Esc.
type WizardCancelledMsg struct{}

// Wizard is the setup wizard overlay.
type Wizard struct {
	step WizardStep

	// Step 1: device
	videoDevices []devices.Device
	audioDevices []devices.Device
	selectedDevice int
	deviceErr    string

	// Step 2-3: OSC
	lastOSCAddr  string   // most recent OSC address seen while listening
	recordAddr   string
	stopAddr     string
	oscHint      string

	// Step 4: output
	outputInput textinput.Model
	prefixInput textinput.Model
	focusedField int

	// Shared
	width  int
	height int
}

// WizardDevicesMsg carries the device list for step 1.
type WizardDevicesMsg struct {
	Video []devices.Device
	Audio []devices.Device
	Err   error
}

// WizardOSCSeenMsg is sent when the wizard sees an OSC packet (used in steps 2-3).
type WizardOSCSeenMsg struct {
	Address string
}

func NewWizard(width, height int, initialOutputDir, initialPrefix string) Wizard {
	out := textinput.New()
	out.Placeholder = "~/Dropbox/recordings"
	out.SetValue(initialOutputDir)
	out.Width = 40
	out.Focus()

	pre := textinput.New()
	pre.Placeholder = "recording"
	pre.SetValue(initialPrefix)
	pre.Width = 20

	return Wizard{
		step:         WizardStepDevice,
		outputInput:  out,
		prefixInput:  pre,
		width:        width,
		height:       height,
	}
}

func (w Wizard) Init() tea.Cmd {
	// Probe devices on init
	return func() tea.Msg {
		// Attempt a simple avfoundation device list (no decklink dependency)
		grps, err := devices.ProbeForPlatform("ffmpeg", "darwin")
		if err != nil {
			return WizardDevicesMsg{Err: err}
		}
		var video, audio []devices.Device
		for _, g := range grps {
			if len(video) == 0 {
				video = g.Video
			}
			if len(audio) == 0 && len(g.Audio) > 0 {
				audio = g.Audio
			}
		}
		return WizardDevicesMsg{Video: video, Audio: audio}
	}
}

func (w Wizard) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {

	case WizardDevicesMsg:
		if msg.Err != nil {
			w.deviceErr = msg.Err.Error()
		} else {
			w.videoDevices = msg.Video
			w.audioDevices = msg.Audio
			if len(w.videoDevices) == 1 {
				// Auto-select the only device
				w.selectedDevice = 0
			}
		}
		return w, nil

	case WizardOSCSeenMsg:
		w.lastOSCAddr = msg.Address
		return w, nil

	case OSCReceivedMsg:
		// Relay OSC into wizard steps 2 and 3
		if w.step == WizardStepOSCRecord || w.step == WizardStepOSCStop {
			w.lastOSCAddr = msg.Address
		}
		return w, nil

	case tea.KeyMsg:
		return w.handleKey(msg)
	}

	// Propagate to text inputs on step 4
	if w.step == WizardStepOutput {
		var cmd tea.Cmd
		if w.focusedField == 0 {
			w.outputInput, cmd = w.outputInput.Update(msg)
		} else {
			w.prefixInput, cmd = w.prefixInput.Update(msg)
		}
		return w, cmd
	}

	return w, nil
}

func (w Wizard) handleKey(msg tea.KeyMsg) (Overlay, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return nil, func() tea.Msg { return WizardCancelledMsg{} }

	case "up", "k":
		if w.step == WizardStepDevice && w.selectedDevice > 0 {
			w.selectedDevice--
		}

	case "down", "j":
		if w.step == WizardStepDevice && w.selectedDevice < len(w.videoDevices)-1 {
			w.selectedDevice++
		}

	case "tab":
		if w.step == WizardStepOutput {
			w.focusedField = 1 - w.focusedField
			if w.focusedField == 0 {
				w.outputInput.Focus()
				w.prefixInput.Blur()
			} else {
				w.outputInput.Blur()
				w.prefixInput.Focus()
			}
		}

	case "enter":
		return w.advance()
	}
	return w, nil
}

func (w Wizard) advance() (Overlay, tea.Cmd) {
	switch w.step {
	case WizardStepDevice:
		if len(w.videoDevices) == 0 {
			w.deviceErr = "No devices found — check capture setup"
			return w, nil
		}
		w.step = WizardStepOSCRecord
		w.lastOSCAddr = ""
		return w, nil

	case WizardStepOSCRecord:
		if w.lastOSCAddr == "" {
			w.oscHint = "No OSC received yet — send your record cue first"
			return w, nil
		}
		w.recordAddr = w.lastOSCAddr
		w.lastOSCAddr = ""
		w.oscHint = ""
		w.step = WizardStepOSCStop
		return w, nil

	case WizardStepOSCStop:
		if w.lastOSCAddr == "" {
			w.oscHint = "No OSC received yet — send your stop cue first"
			return w, nil
		}
		w.stopAddr = w.lastOSCAddr
		w.lastOSCAddr = ""
		w.oscHint = ""
		w.step = WizardStepOutput
		w.outputInput.Focus()
		return w, nil

	case WizardStepOutput:
		deviceName := ""
		if len(w.videoDevices) > 0 {
			deviceName = w.videoDevices[w.selectedDevice].Name
		}
		result := WizardResult{
			DeviceName:    deviceName,
			RecordAddress: w.recordAddr,
			StopAddress:   w.stopAddr,
			OutputDir:     w.outputInput.Value(),
			Prefix:        w.prefixInput.Value(),
		}
		return nil, func() tea.Msg { return WizardDoneMsg{Result: result} }
	}
	return w, nil
}

func (w Wizard) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("SETUP WIZARD") + "\n")

	steps := []string{"Device", "Record cue", "Stop cue", "Output"}
	stepLine := ""
	for i, s := range steps {
		st := WizardStep(i)
		if st < w.step {
			stepLine += styleLocked.Render("✓ "+s) + "  "
		} else if st == w.step {
			stepLine += styleWarning.Render("→ "+s) + "  "
		} else {
			stepLine += styleDim.Render("  "+s) + "  "
		}
	}
	b.WriteString(stepLine + "\n\n")

	switch w.step {
	case WizardStepDevice:
		b.WriteString(styleText.Render("Select your capture device:") + "\n\n")
		if w.deviceErr != "" {
			b.WriteString(styleError.Render("  ✗ "+w.deviceErr) + "\n")
		} else if len(w.videoDevices) == 0 {
			b.WriteString(styleDim.Render("  Probing devices...") + "\n")
		} else {
			for i, d := range w.videoDevices {
				prefix := "  "
				if i == w.selectedDevice {
					prefix = styleWarning.Render("▶ ")
				}
				b.WriteString(prefix + styleText.Render(d.Name) + "\n")
			}
		}
		b.WriteString("\n" + styleDim.Render("[↑↓] select  [Enter] confirm  [Esc] cancel"))

	case WizardStepOSCRecord:
		b.WriteString(styleText.Render("Send your RECORD cue now:") + "\n\n")
		if w.lastOSCAddr != "" {
			b.WriteString(styleLocked.Render("  ● "+w.lastOSCAddr) + "\n")
		} else {
			b.WriteString(styleDim.Render("  Listening on :8000 — waiting for OSC...") + "\n")
		}
		if w.oscHint != "" {
			b.WriteString("\n" + styleWarning.Render("  "+w.oscHint) + "\n")
		}
		b.WriteString("\n" + styleDim.Render("[Enter] select  [Esc] cancel"))

	case WizardStepOSCStop:
		b.WriteString(styleText.Render("Send your STOP cue now:") + "\n\n")
		b.WriteString(styleDim.Render("  Record: ")+styleLocked.Render(w.recordAddr)+"\n\n")
		if w.lastOSCAddr != "" {
			b.WriteString(styleLocked.Render("  ● "+w.lastOSCAddr) + "\n")
		} else {
			b.WriteString(styleDim.Render("  Listening on :8000 — waiting for OSC...") + "\n")
		}
		if w.oscHint != "" {
			b.WriteString("\n" + styleWarning.Render("  "+w.oscHint) + "\n")
		}
		b.WriteString("\n" + styleDim.Render("[Enter] select  [Esc] cancel"))

	case WizardStepOutput:
		b.WriteString(styleText.Render("Output configuration:") + "\n\n")
		b.WriteString(fmt.Sprintf("  %-12s %s\n", "Directory:", w.outputInput.View()))
		b.WriteString(fmt.Sprintf("  %-12s %s\n", "Prefix:", w.prefixInput.View()))
		b.WriteString("\n" + styleDim.Render("[Tab] switch field  [Enter] save  [Esc] cancel"))
	}

	content := b.String()
	return PanelStyle(64).Render(content)
}

func (w Wizard) Size() (int, int) {
	return 68, 18
}

// WizardFormatDevices is a helper for rendering device list in non-overlay contexts.
func formatDeviceTable(devs []devices.Device) string {
	if len(devs) == 0 {
		return styleDim.Render("  (none found)")
	}
	var b strings.Builder
	for i, d := range devs {
		b.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Render(d.Name)))
	}
	return b.String()
}
