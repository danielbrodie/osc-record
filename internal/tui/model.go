package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	minWidth  = 100
	minHeight = 30
)

// RecordingState tracks the current recording state machine.
type RecordingState int

const (
	StateIdle RecordingState = iota
	StateStarting
	StateRecording
	StateStopping
	StateError
)

func (s RecordingState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateStarting:
		return "STARTING"
	case StateRecording:
		return "RECORDING"
	case StateStopping:
		return "STOPPING"
	case StateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ClipInfo holds metadata about a recorded clip in this session.
type ClipInfo struct {
	Index     int
	File      string
	Device    string
	StartTime time.Time
	Duration  time.Duration
	SizeBytes int64
	Verified  *bool  // nil = pending, true = ok, false = failed
	VerifyErr []string
}

// Model is the root bubbletea model for the TUI.
type Model struct {
	keys   KeyMap
	width  int
	height int
	cmdCh  chan UserCmd

	// Sub-models (panels)
	oscPanel    OSCPanel
	signalPanel SignalPanel
	statusPanel StatusPanel
	clipsPanel  ClipsPanel
	logPanel    LogPanel

	// Active overlay (nil if none)
	overlay Overlay

	// Application state
	recordState RecordingState
	currentFile string
	startTime   time.Time
	fileSize    int64

	// Signal state
	signalLocked bool
	signalFormat string
	signalInput  string
	signalRes    string
	signalFPS    string
	audioLeft    float64
	audioRight   float64
	timecode     string

	// Session clips
	clips []ClipInfo

	// Disk
	diskFree  uint64
	diskTotal uint64
	diskPath  string

	// Error banner
	banner string

	// Blink state (500ms tick)
	blink bool

	// Config references (passed in, not owned)
	recordAddr string
	stopAddr   string
	deviceName string
}

// Overlay is an interface for overlay panels (wizard, scanner, checklist, etc.)
type Overlay interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Overlay, tea.Cmd)
	View() string
	// Size returns the width and height the overlay wants.
	Size() (int, int)
}

// New creates the root TUI model.
func New(recordAddr, stopAddr, deviceName string) Model {
	m := Model{
		keys:        DefaultKeyMap(),
		cmdCh:       make(chan UserCmd, 8),
		recordAddr:  recordAddr,
		stopAddr:    stopAddr,
		deviceName:  deviceName,
		audioLeft:   -60,
		audioRight:  -60,
	}
	m.oscPanel = NewOSCPanel(recordAddr, stopAddr)
	m.signalPanel = NewSignalPanel()
	m.statusPanel = NewStatusPanel()
	m.clipsPanel = NewClipsPanel()
	m.logPanel = NewLogPanel()
	return m
}

func (m Model) Commands() <-chan UserCmd {
	return m.cmdCh
}

// Init starts background ticks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.oscPanel.Init(),
		m.signalPanel.Init(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle overlay first — it captures most messages.
	if m.overlay != nil {
		switch msg.(type) {
		case tea.KeyMsg:
			// Let overlay handle keys; check for Esc to dismiss.
		}
		newOverlay, cmd := m.overlay.Update(msg)
		cmds = append(cmds, cmd)
		if newOverlay == nil {
			m.overlay = nil
		} else {
			m.overlay = newOverlay
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()

	case TickMsg:
		m.blink = !m.blink
		cmds = append(cmds, tickCmd())
		// Update elapsed duration display
		if m.recordState == StateRecording {
			m.statusPanel.Elapsed = time.Since(m.startTime)
		}

	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		cmds = append(cmds, cmd)

	case SignalStateMsg:
		m.signalLocked = msg.Locked
		m.signalFormat = msg.Format
		m.signalInput = msg.Input
		m.signalRes = msg.Resolution
		m.signalFPS = msg.FPS
		m.signalPanel.Update(msg)
		if msg.Locked {
			m.addLog("Signal locked: " + msg.Input + " " + msg.Resolution + " " + msg.FPS + "fps " + msg.Format)
		} else if msg.Err != "" {
			m.addLog("Signal: " + msg.Err)
		}

	case AudioLevelMsg:
		m.audioLeft = msg.Left
		m.audioRight = msg.Right
		m.signalPanel.UpdateAudio(msg)

	case TimecodeMsg:
		m.timecode = msg.TC
		m.signalPanel.UpdateTC(msg)

	case OSCReceivedMsg:
		m.oscPanel.Append(msg)

	case RecordingStartedMsg:
		m.recordState = StateRecording
		m.currentFile = msg.File
		m.startTime = msg.Time
		m.fileSize = 0
		clip := ClipInfo{
			Index:     len(m.clips) + 1,
			File:      msg.File,
			Device:    msg.Device,
			StartTime: msg.Time,
		}
		m.clips = append(m.clips, clip)
		m.clipsPanel.SetClips(m.clips)
		m.statusPanel.SetRecording(msg.File, msg.Time)
		m.addLog("Recording started: " + msg.File)

	case RecordingStoppedMsg:
		m.recordState = StateIdle
		m.updateClip(msg.File, func(c *ClipInfo) {
			c.Duration = msg.Duration
			c.SizeBytes = msg.SizeBytes
		})
		m.clipsPanel.SetClips(m.clips)
		m.statusPanel.SetIdle()
		m.addLog(fmt.Sprintf("Recording saved: %s (%s)", msg.File, fmtDuration(msg.Duration)))

	case RecordingCrashedMsg:
		m.recordState = StateError
		m.addLog(fmt.Sprintf("ffmpeg crashed: %v — partial clip: %s", msg.Err, msg.File))
		if msg.Recoverable {
			m.banner = "Recording crashed — attempting recovery..."
		} else {
			m.banner = "Recording crashed — manual intervention required"
		}

	case RecordingResumedMsg:
		m.recordState = StateIdle
		m.banner = ""
		m.addLog("Capture resumed after crash")

	case FileSizeMsg:
		m.fileSize = msg.SizeBytes
		m.updateClip(msg.File, func(c *ClipInfo) { c.SizeBytes = msg.SizeBytes })
		m.statusPanel.FileSize = msg.SizeBytes

	case ClipVerifiedMsg:
		ok := msg.OK
		m.updateClip(msg.File, func(c *ClipInfo) {
			c.Verified = &ok
			c.VerifyErr = msg.Errors
		})
		m.clipsPanel.SetClips(m.clips)
		if msg.OK {
			m.addLog("Clip verified: " + msg.File)
		} else {
			m.addLog("Clip FAILED verification: " + msg.File + " — " + joinStrings(msg.Errors))
			m.banner = "Clip verification failed: " + msg.File
		}

	case DiskStatMsg:
		m.diskFree = msg.FreeBytes
		m.diskTotal = msg.TotalBytes
		m.diskPath = msg.Path
		m.statusPanel.SetDisk(msg)

	case LogMsg:
		m.logPanel.Append(msg)

	case ErrorBannerMsg:
		m.banner = msg.Text

	case ClearBannerMsg:
		m.banner = ""
	}

	// Propagate to sub-panels that need independent update cycles
	var cmd tea.Cmd
	m.logPanel, cmd = m.logPanel.Update(msg)
	cmds = append(cmds, cmd)
	m.oscPanel, cmd = m.oscPanel.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Quit):
		if m.recordState == StateRecording {
			// TODO: show quit confirmation overlay
		}
		return tea.Quit

	case key.Matches(msg, m.keys.Escape):
		m.banner = ""

	case key.Matches(msg, m.keys.Record):
		// Manual override — send a synthetic record trigger
		if m.recordState == StateIdle {
			m.recordState = StateStarting
			m.emitCommand(UserCmdRecord)
		}

	case key.Matches(msg, m.keys.Stop):
		if m.recordState == StateRecording {
			m.recordState = StateStopping
			m.emitCommand(UserCmdStop)
		}

	case key.Matches(msg, m.keys.Help):
		// TODO: help overlay

	case key.Matches(msg, m.keys.Checklist):
		// TODO: launch checklist overlay

	case key.Matches(msg, m.keys.Scanner):
		// TODO: launch scanner overlay

	case key.Matches(msg, m.keys.Wizard):
		// TODO: launch wizard overlay
	}
	return nil
}

func (m *Model) emitCommand(cmd UserCmd) {
	select {
	case m.cmdCh <- cmd:
	default:
	}
}

// View renders the full TUI screen.
func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return m.viewResize()
	}

	if m.overlay != nil {
		return m.viewWithOverlay()
	}

	return m.viewMain()
}

func (m Model) viewResize() string {
	msg := styleWarning.Render(fmt.Sprintf(
		"Terminal too small — minimum %dx%d, current %dx%d",
		minWidth, minHeight, m.width, m.height,
	))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
}

func (m Model) viewWithOverlay() string {
	// Render overlay centered over main content
	ov := m.overlay.View()
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ov)
}

func (m Model) viewMain() string {
	// Layout:
	// [signal | osc   ]
	// [signal | clips ]
	// [status         ]
	// [log            ]
	// [keys           ]

	leftW := 42
	rightW := m.width - leftW - 3 // 3 for border/gap

	topH := 9  // signal + osc panels
	statusH := 4
	logH := m.height - topH - statusH - 3 // 3 for key bar + border

	sig := m.signalPanel.View(leftW, topH)
	osc := m.oscPanel.View(rightW, topH/2+1)
	clips := m.clipsPanel.View(rightW, topH/2)
	right := lipgloss.JoinVertical(lipgloss.Left, osc, clips)
	top := lipgloss.JoinHorizontal(lipgloss.Top, sig, " ", right)

	status := m.statusPanel.View(m.width-2, statusH, m.blink)
	log := m.logPanel.View(m.width-2, logH)

	// Error banner
	var bannerLine string
	if m.banner != "" {
		bannerLine = styleBanner.Width(m.width - 4).Render("⚠  " + m.banner) + "\n"
	}

	keys := KeyHints(
		"R", "Record",
		"S", "Stop",
		"P", "Preview",
		"F1", "Scan",
		"F2", "Checklist",
		"W", "Setup",
		"?", "Help",
		"Q", "Quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		top,
		status,
		bannerLine+log,
		styleKeyHint.Render(keys),
	)
}

func (m *Model) relayout() {
	// Notify panels of new dimensions — they recalculate on next View() call.
}

func (m *Model) addLog(text string) {
	m.logPanel.Append(LogMsg{Time: time.Now(), Text: text})
}

func (m *Model) updateClip(file string, fn func(*ClipInfo)) {
	for i := range m.clips {
		if m.clips[i].File == file {
			fn(&m.clips[i])
			return
		}
	}
}

// Helpers

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += "; "
		}
		result += s
	}
	return result
}
