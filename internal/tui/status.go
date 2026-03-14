package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type DeviceStatus struct {
	Device      string
	CaptureMode string
	FormatCode  string
	State       RecordingState
	File        string
	StartTime   time.Time
	Elapsed     time.Duration
	FileSize    int64
}

// StatusPanel renders the recording status bar.
type StatusPanel struct {
	Devices []DeviceStatus

	DiskFree  uint64
	DiskTotal uint64
	DiskPath  string

	BitrateBps float64 // estimated from filesize/elapsed
}

func NewStatusPanel(deviceNames []string) StatusPanel {
	panel := StatusPanel{}
	panel.SetDevices(deviceNames)
	return panel
}

func (p *StatusPanel) SetDevices(deviceNames []string) {
	if len(deviceNames) == 0 {
		deviceNames = []string{""}
	}

	existing := make(map[string]DeviceStatus, len(p.Devices))
	for _, device := range p.Devices {
		existing[device.Device] = device
	}

	devices := make([]DeviceStatus, 0, len(deviceNames))
	for _, name := range deviceNames {
		device := existing[name]
		device.Device = name
		if device.State == 0 {
			device.State = StateIdle
		}
		devices = append(devices, device)
	}
	p.Devices = devices
}

func (p *StatusPanel) SetDeviceConfig(deviceName, captureMode, formatCode string) {
	device := p.ensureDevice(deviceName)
	device.CaptureMode = captureMode
	device.FormatCode = formatCode
}

func (p *StatusPanel) SetRecording(deviceName, file string, startedAt time.Time) {
	device := p.ensureDevice(deviceName)
	device.State = StateRecording
	device.File = file
	device.StartTime = startedAt
	device.Elapsed = 0
	device.FileSize = 0
}

func (p *StatusPanel) SetIdle(deviceName string) {
	device := p.ensureDevice(deviceName)
	device.State = StateIdle
	device.File = ""
	device.StartTime = time.Time{}
	device.Elapsed = 0
	device.FileSize = 0
}

func (p *StatusPanel) SetError(deviceName string) {
	device := p.ensureDevice(deviceName)
	device.State = StateError
}

func (p *StatusPanel) SetFileSize(file string, sizeBytes int64) {
	for i := range p.Devices {
		if p.Devices[i].File == file {
			p.Devices[i].FileSize = sizeBytes
			return
		}
	}
}

func (p *StatusPanel) Tick(now time.Time) {
	for i := range p.Devices {
		if p.Devices[i].State == StateRecording && !p.Devices[i].StartTime.IsZero() {
			p.Devices[i].Elapsed = now.Sub(p.Devices[i].StartTime)
		}
	}
}

func (p *StatusPanel) SetDisk(msg DiskStatMsg) {
	p.DiskFree = msg.FreeBytes
	p.DiskTotal = msg.TotalBytes
	p.DiskPath = msg.Path
}

func (p StatusPanel) Height() int {
	lines := len(p.Devices) + 2
	if len(p.Devices) <= 1 {
		lines = 3
	}
	return lines + 1
}

func (p StatusPanel) AnyRecording() bool {
	for _, device := range p.Devices {
		if device.State == StateRecording {
			return true
		}
	}
	return false
}

func (p StatusPanel) View(width, height int, blink bool) string {
	content := []string{}
	if len(p.Devices) <= 1 {
		device := DeviceStatus{State: StateIdle}
		if len(p.Devices) == 1 {
			device = p.Devices[0]
		}

		line1 := p.renderState(device.State, blink)
		if device.File != "" {
			line1 += "   " + styleText.Render(device.File) +
				"   " + styleText.Render(fmtDuration(device.Elapsed)) +
				"   " + styleText.Render(fmtBytes(device.FileSize))
		}

		deviceInfo := styleDim.Render(device.Device)
		if device.CaptureMode != "" {
			deviceInfo += styleDim.Render("  " + device.CaptureMode)
		}
		if device.FormatCode != "" {
			deviceInfo += styleDim.Render("  " + device.FormatCode)
		}

		content = append(content, line1, deviceInfo+p.renderDiskInfo())
	} else {
		for _, device := range p.Devices {
			line := fmt.Sprintf(
				"%-14s  %-20s  %-8s  %5s  %6s  %s",
				p.renderState(device.State, blink),
				shortName(device.Device, 20),
				shortName(device.File, 8),
				fmtDuration(device.Elapsed),
				fmtBytes(device.FileSize),
				styleDim.Render(joinNonEmpty(device.CaptureMode, device.FormatCode)),
			)
			content = append(content, styleText.Render(line))
		}
		if disk := p.renderDiskOnly(); disk != "" {
			content = append(content, disk)
		}
	}

	return PanelStyle(width).Render(
		lipgloss.JoinVertical(lipgloss.Left, TitleBar("STATUS"), lipgloss.JoinVertical(lipgloss.Left, content...)),
	)
}

func (p *StatusPanel) ensureDevice(deviceName string) *DeviceStatus {
	for i := range p.Devices {
		if p.Devices[i].Device == deviceName {
			return &p.Devices[i]
		}
	}

	p.Devices = append(p.Devices, DeviceStatus{
		Device: deviceName,
		State:  StateIdle,
	})
	return &p.Devices[len(p.Devices)-1]
}

func (p StatusPanel) renderState(state RecordingState, blink bool) string {
	switch state {
	case StateRecording:
		dot := "●"
		if !blink {
			dot = "○"
		}
		return styleRecording.Render(dot + " RECORDING")
	case StateStarting:
		return styleWarning.Render("◌ STARTING")
	case StateStopping:
		return styleWarning.Render("◌ STOPPING")
	case StateError:
		return styleError.Render("✗ ERROR")
	default:
		return styleIdle.Render("○ IDLE")
	}
}

func (p StatusPanel) renderDiskInfo() string {
	if p.DiskFree == 0 {
		return ""
	}

	remaining := p.estimateRemaining()
	return "   " + styleText.Render(p.DiskPath) + "   " +
		styleText.Render(fmtBytesHuman(p.DiskFree)+" free") +
		styleDim.Render("  "+remaining)
}

func (p StatusPanel) renderDiskOnly() string {
	if p.DiskFree == 0 {
		return ""
	}
	return styleText.Render("Disk: "+p.DiskPath+"   "+fmtBytesHuman(p.DiskFree)+" free") +
		styleDim.Render("  "+p.estimateRemaining())
}

func (p StatusPanel) estimateRemaining() string {
	if p.DiskFree == 0 {
		return ""
	}
	if p.BitrateBps <= 0 || !p.AnyRecording() {
		return "~" + fmtBytesHuman(p.DiskFree) + " free"
	}
	secondsRemaining := float64(p.DiskFree) / p.BitrateBps
	hours := int(secondsRemaining / 3600)
	if hours > 999 {
		hours = 999
	}
	return fmt.Sprintf("~%dh remaining at current rate", hours)
}

func fmtBytes(b int64) string {
	if b == 0 {
		return "—"
	}
	return fmtBytesHuman(uint64(b))
}

func fmtBytesHuman(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.0fMB", float64(b)/mb)
	case b >= kb:
		return fmt.Sprintf("%.0fKB", float64(b)/kb)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func joinNonEmpty(parts ...string) string {
	result := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if result != "" {
			result += "  "
		}
		result += part
	}
	return result
}
