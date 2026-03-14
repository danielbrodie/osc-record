package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/danielbrodie/osc-record/internal/capture"
	"github.com/danielbrodie/osc-record/internal/devices"
)

type ChecklistConfig struct {
	FFmpegPath    string
	DeviceName    string
	FormatCode    string
	OutputDir     string
	CaptureMode   string
	RecordAddress string
	StopAddress   string
}

type ChecklistOverlay struct {
	cfg     ChecklistConfig
	results []CheckResult
}

func NewChecklist(cfg ChecklistConfig) *ChecklistOverlay {
	return &ChecklistOverlay{cfg: cfg}
}

func (c *ChecklistOverlay) Init() tea.Cmd {
	return tea.Batch(
		checkDeviceFound(c.cfg),
		checkDriverActive(),
		checkSignalLocked(c.cfg),
		checkOSCConfigured(c.cfg),
		checkOutputDir(c.cfg),
		checkDiskSpace(c.cfg),
		checkFFmpegDecklink(c.cfg),
	)
}

func (c *ChecklistOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return nil, nil
		}
	case ChecklistResultMsg:
		c.results = appendChecklistResults(c.results, msg.Results)
	}
	return c, nil
}

func (c *ChecklistOverlay) View() string {
	lines := []string{
		TitleBar("PRE-SHOW CHECKLIST"),
		"",
		styleDim.Render("Esc to close"),
	}

	passed := 0
	for _, result := range c.results {
		icon := styleError.Render("✗")
		if result.OK {
			icon = styleLocked.Render("✓")
			passed++
		}

		detail := result.Detail
		if detail == "" {
			detail = "ok"
		}

		lines = append(lines, fmt.Sprintf("%s %s  %s", icon, styleText.Render(result.Name), styleDim.Render(detail)))
		if result.OK {
			lines = append(lines, styleDim.Render("  Ready"))
		} else {
			lines = append(lines, styleWarning.Render("  "+result.Fix))
		}
	}

	if len(c.results) == 0 {
		lines = append(lines, styleDim.Render("Running checks..."))
	}

	lines = append(lines, "")
	lines = append(lines, styleText.Render(fmt.Sprintf("Passed %d/%d", passed, len(checklistKeys))))
	return PanelStyle(80).Render(strings.Join(lines, "\n"))
}

func (c *ChecklistOverlay) Size() (int, int) {
	return 80, 7 + len(checklistKeys)*2
}

var checklistKeys = []string{
	"device-found",
	"driver-active",
	"signal-locked",
	"osc-configured",
	"output-dir",
	"disk-space",
	"ffmpeg-decklink",
}

func appendChecklistResults(existing []CheckResult, incoming []CheckResult) []CheckResult {
	for _, result := range incoming {
		replaced := false
		for i := range existing {
			if existing[i].Name == result.Name {
				existing[i] = result
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, result)
		}
	}
	return existing
}

func checkDeviceFound(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "Device found", Fix: "Connect the capture hardware and confirm ffmpeg can list it."}
		groups, err := devices.ProbeForPlatform(cfg.FFmpegPath, runtime.GOOS)
		if err != nil {
			result.Detail = err.Error()
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		for _, group := range groups {
			if len(group.Video) > 0 {
				result.OK = true
				result.Detail = fmt.Sprintf("%d device(s) available", len(group.Video))
				return ChecklistResultMsg{Results: []CheckResult{result}}
			}
		}

		result.Detail = "No capture devices found"
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkDriverActive() tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "Driver active", Fix: "Open Desktop Video Setup and confirm Blackmagic Driver Extension is activated."}
		if runtime.GOOS != "darwin" {
			result.OK = true
			result.Detail = "Not required on this platform"
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		output, err := exec.Command("systemextensionsctl", "list").CombinedOutput()
		if err != nil {
			result.Detail = strings.TrimSpace(string(output))
			if result.Detail == "" {
				result.Detail = err.Error()
			}
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		text := string(output)
		if strings.Contains(text, "BlackmagicIO.DExt") && strings.Contains(text, "activated enabled") {
			result.OK = true
			result.Detail = "Blackmagic driver extension active"
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		result.Detail = "Blackmagic driver extension not active"
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkSignalLocked(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "Signal locked", Fix: "Confirm SDI/HDMI feed, device selection, and format code."}
		if cfg.CaptureMode != capture.ModeDecklink {
			result.OK = true
			result.Detail = "Signal probe skipped for non-decklink mode"
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}
		if cfg.DeviceName == "" {
			result.Detail = "No capture device selected"
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		args := []string{"-hide_banner", "-f", "decklink"}
		if cfg.FormatCode != "" {
			args = append(args, "-format_code", cfg.FormatCode)
		}
		args = append(args, "-i", cfg.DeviceName, "-t", "1", "-f", "null", "-")

		cmd := exec.Command(cfg.FFmpegPath, args...)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			result.Detail = tailLine(output.String(), err.Error())
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		result.OK = true
		result.Detail = "Signal probe passed"
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkOSCConfigured(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "OSC configured", Fix: "Set both record and stop OSC addresses before showtime."}
		result.OK = cfg.RecordAddress != "" && cfg.StopAddress != ""
		if result.OK {
			result.Detail = cfg.RecordAddress + " / " + cfg.StopAddress
		} else {
			result.Detail = "Record and stop triggers not both configured"
		}
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkOutputDir(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "Output dir writable", Fix: "Choose a writable output directory with enough permissions."}
		if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
			result.Detail = err.Error()
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}
		file, err := os.CreateTemp(cfg.OutputDir, "osc-record-check-*")
		if err != nil {
			result.Detail = err.Error()
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)

		result.OK = true
		result.Detail = cfg.OutputDir
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkDiskSpace(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "Disk space", Fix: "Free at least 5GB in the output directory volume."}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(cfg.OutputDir, &stat); err != nil {
			result.Detail = err.Error()
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}

		freeBytes := stat.Bavail * uint64(stat.Bsize)
		result.OK = freeBytes > 5*1024*1024*1024
		result.Detail = fmtBytesHuman(freeBytes) + " free"
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func checkFFmpegDecklink(cfg ChecklistConfig) tea.Cmd {
	return func() tea.Msg {
		result := CheckResult{Name: "ffmpeg decklink", Fix: "Install an ffmpeg build with decklink support."}
		cmd := exec.Command(cfg.FFmpegPath, "-sources", "decklink")
		output, err := cmd.CombinedOutput()
		text := string(output)
		if err == nil && !strings.Contains(text, "Unknown input format") {
			result.OK = true
			result.Detail = "decklink input available"
			return ChecklistResultMsg{Results: []CheckResult{result}}
		}
		if strings.TrimSpace(text) == "" && err != nil {
			result.Detail = err.Error()
		} else {
			result.Detail = tailLine(text, "decklink source check failed")
		}
		return ChecklistResultMsg{Results: []CheckResult{result}}
	}
}

func tailLine(output, fallback string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return fallback
}
