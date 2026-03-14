package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	goosc "github.com/hypebeast/go-osc/osc"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/danielbrodie/osc-record/internal/capture"
	cfgpkg "github.com/danielbrodie/osc-record/internal/config"
	"github.com/danielbrodie/osc-record/internal/diskmon"
	"github.com/danielbrodie/osc-record/internal/devices"
	"github.com/danielbrodie/osc-record/internal/health"
	oscpkg "github.com/danielbrodie/osc-record/internal/osc"
	"github.com/danielbrodie/osc-record/internal/platform"
	"github.com/danielbrodie/osc-record/internal/recorder"
	"github.com/danielbrodie/osc-record/internal/sigpoll"
	"github.com/danielbrodie/osc-record/internal/tui"
	"github.com/danielbrodie/osc-record/internal/verifier"
)

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func init() {
	defaults := cfgpkg.Defaults()

	runCmd.Flags().String("prefix", defaults.Recording.Prefix, "Filename prefix prepended to date")
	runCmd.Flags().String("profile", defaults.Recording.Profile, "Recording profile: prores, hevc, or h264")
	runCmd.Flags().String("output", defaults.Recording.OutputDir, "Output directory for recordings")
	runCmd.Flags().Int("port", defaults.OSC.Port, "Override OSC listen port")
	runCmd.Flags().String("capture-mode", defaults.Device.CaptureMode, "Capture mode: auto, decklink, avfoundation, or dshow")
	runCmd.Flags().String("video-device", "", "Override video device (index or name)")
	runCmd.Flags().String("audio-device", "", "Override audio device (index or name)")
	runCmd.Flags().Bool("no-tui", false, "Force plaintext mode even in a TTY")
	runCmd.Flags().Int("http-port", 0, "Enable HTTP status endpoint on this port")
	runCmd.Flags().Int("pre-roll", 0, "Pre-roll buffer in seconds (0 = disabled)")

	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the OSC recording daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := mustConfig()

		ffmpegPath, err := resolveFFmpegPath(cfg)
		if err != nil {
			return err
		}

		cfg, err = applyRunFlagOverrides(cmd, cfg)
		if err != nil {
			return err
		}

		noTUI, _ := cmd.Flags().GetBool("no-tui")
		if isTTY() && cfg.TUI.Enabled && !noTUI {
			return runTUI(cfg, ffmpegPath, cmd)
		}
		return runPlaintext(cfg, ffmpegPath, cmd)
	},
}

type selectedDevices struct {
	VideoDisplay     string
	VideoConfigValue string
	AudioDisplay     string
	AudioConfigValue string
}

func applyRunFlagOverrides(cmd *cobra.Command, cfg cfgpkg.Config) (cfgpkg.Config, error) {
	if cmd.Flags().Changed("prefix") {
		value, _ := cmd.Flags().GetString("prefix")
		cfg.Recording.Prefix = value
	}
	if cmd.Flags().Changed("profile") {
		value, _ := cmd.Flags().GetString("profile")
		if !recorder.ValidProfile(value) {
			return cfg, fmt.Errorf("Error: Invalid profile %q. Use prores, hevc, or h264.", value)
		}
		cfg.Recording.Profile = value
	}
	if cmd.Flags().Changed("output") {
		value, _ := cmd.Flags().GetString("output")
		cfg.Recording.OutputDir = value
	}
	if cmd.Flags().Changed("port") {
		value, _ := cmd.Flags().GetInt("port")
		cfg.OSC.Port = value
	}
	if cmd.Flags().Changed("capture-mode") {
		value, _ := cmd.Flags().GetString("capture-mode")
		cfg.Device.CaptureMode = value
	}
	if cmd.Flags().Changed("video-device") {
		value, _ := cmd.Flags().GetString("video-device")
		cfg.Device.Name = value
	}
	if cmd.Flags().Changed("audio-device") {
		value, _ := cmd.Flags().GetString("audio-device")
		cfg.Device.Audio = value
	}
	return cfg, nil
}

func ensureDevicesConfigured(ffmpegPath string, mode capture.CaptureMode, cfg cfgpkg.Config, videoOverride, audioOverride bool) (selectedDevices, cfgpkg.Config, bool, error) {
	var changed bool

	group, err := devices.ProbeMode(ffmpegPath, mode.Name())
	if err != nil {
		return selectedDevices{}, cfg, false, err
	}

	selected := selectedDevices{}
	if cfg.Device.Name == "" && !videoOverride {
		video, err := promptForDevice(group.Video, "capture device", mode.Name() == capture.ModeDecklink)
		if err != nil {
			return selectedDevices{}, cfg, false, err
		}
		cfg.Device.Name = video.ConfigValue()
		selected.VideoDisplay = video.Name
		selected.VideoConfigValue = video.ConfigValue()
		changed = true
	} else {
		video, err := devices.MatchDevice(group.Video, cfg.Device.Name)
		if err != nil {
			return selectedDevices{}, cfg, false, fmt.Errorf("Error: Video device %q not found. Run 'osc-record devices' to list available devices.", cfg.Device.Name)
		}
		selected.VideoDisplay = video.Name
		selected.VideoConfigValue = cfg.Device.Name
	}

	if mode.NeedsAudio() {
		if cfg.Device.Audio == "" && !audioOverride {
			audio, err := promptForDevice(group.Audio, "audio device", false)
			if err != nil {
				return selectedDevices{}, cfg, false, err
			}
			cfg.Device.Audio = audio.ConfigValue()
			selected.AudioDisplay = audio.Name
			selected.AudioConfigValue = audio.ConfigValue()
			changed = true
		} else {
			audio, err := devices.MatchDevice(group.Audio, cfg.Device.Audio)
			if err != nil {
				return selectedDevices{}, cfg, false, fmt.Errorf("Error: Audio device %q not found. Run 'osc-record devices' to list available devices.", cfg.Device.Audio)
			}
			selected.AudioDisplay = audio.Name
			selected.AudioConfigValue = cfg.Device.Audio
		}
	}

	return selected, cfg, changed, nil
}

func promptForDevice(items []devices.Device, label string, singlePrompt bool) (devices.Device, error) {
	if len(items) == 0 {
		return devices.Device{}, errors.New("Error: No capture devices found. Run 'osc-record devices' for details.")
	}
	if len(items) == 1 {
		fmt.Printf("Auto-selected device: %s\n", items[0].Name)
		return items[0], nil
	}

	reader := bufio.NewReader(os.Stdin)
	if singlePrompt {
		fmt.Println("No capture device configured. Available devices:")
	} else if strings.Contains(label, "audio") {
		fmt.Println("Available audio devices:")
	} else {
		fmt.Println("No capture device configured. Available video devices:")
	}
	fmt.Println()

	for i, item := range items {
		fmt.Printf("  [%d] %s\n", i+1, item.Name)
	}
	fmt.Println()

	for {
		switch {
		case strings.Contains(label, "audio"):
			fmt.Printf("Select audio device [1-%d]: ", len(items))
		case singlePrompt:
			fmt.Printf("Select device [1-%d]: ", len(items))
		default:
			fmt.Printf("Select video device [1-%d]: ", len(items))
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return devices.Device{}, err
		}
		index, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || index < 1 || index > len(items) {
			fmt.Println("Invalid selection.")
			continue
		}
		return items[index-1], nil
	}
}

func runTUI(cfg cfgpkg.Config, ffmpegPath string, cmd *cobra.Command) error {
	mode, _, err := capture.ResolveMode(cfg.Device.CaptureMode, ffmpegPath, runtime.GOOS, cfg.Device.FormatCode)
	if err != nil {
		return err
	}

	deviceInfo, updatedCfg, cfgChanged, err := ensureDevicesConfigured(ffmpegPath, mode, cfg, cmd.Flags().Changed("video-device"), cmd.Flags().Changed("audio-device"))
	if err != nil {
		return err
	}
	cfg = updatedCfg
	if cfgChanged {
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}

	outDir := outputDir(cfg)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("Error: Output directory %s does not exist and could not be created: %v.", outDir, err)
	}

	model := tui.New(cfg.OSC.RecordAddress, cfg.OSC.StopAddress, deviceInfo.VideoDisplay)
	model.SetChecklistConfig(tui.ChecklistConfig{
		FFmpegPath:    ffmpegPath,
		DeviceName:    deviceInfo.VideoDisplay,
		FormatCode:    cfg.Device.FormatCode,
		OutputDir:     outDir,
		CaptureMode:   mode.Name(),
		RecordAddress: cfg.OSC.RecordAddress,
		StopAddress:   cfg.OSC.StopAddress,
	})
	commandCh := model.Commands()
	p := tea.NewProgram(model, tea.WithAltScreen())
	oscCh := make(chan tuiOSCMessage, 32)
	clipVerifier := verifier.Verifier{}

	var (
		statusMu     sync.Mutex
		status       = health.StatusSnapshot{
			State:         tui.StateIdle.String(),
			Device:        deviceInfo.VideoDisplay,
			Format:        cfg.Device.FormatCode,
			OSCPort:       cfg.OSC.Port,
			RecordAddress: cfg.OSC.RecordAddress,
			StopAddress:   cfg.OSC.StopAddress,
		}
		sessionClips []health.ClipInfo
	)

	sendToUI := func(msg tea.Msg) {
		statusMu.Lock()
		switch msg := msg.(type) {
		case tui.SignalStateMsg:
			status.SignalLocked = msg.Locked
			if msg.Format != "" {
				status.Format = msg.Format
			}
		case tui.DiskStatMsg:
			status.DiskFreeBytes = msg.FreeBytes
		case tui.RecordingStartedMsg:
			status.State = tui.StateRecording.String()
			status.File = msg.File
			status.SizeBytes = 0
			status.DurationSec = 0
			sessionClips = append(sessionClips, health.ClipInfo{
				Index:     len(sessionClips) + 1,
				File:      msg.File,
				Device:    msg.Device,
				StartTime: msg.Time,
			})
			status.ClipsThisSession = len(sessionClips)
		case tui.FileSizeMsg:
			status.SizeBytes = msg.SizeBytes
			for i := range sessionClips {
				if sessionClips[i].File == msg.File {
					sessionClips[i].SizeBytes = msg.SizeBytes
				}
			}
		case tui.RecordingStoppedMsg:
			status.State = tui.StateIdle.String()
			status.File = msg.File
			status.SizeBytes = msg.SizeBytes
			status.DurationSec = int(msg.Duration.Seconds())
			for i := range sessionClips {
				if sessionClips[i].File == msg.File {
					sessionClips[i].Duration = msg.Duration
					sessionClips[i].SizeBytes = msg.SizeBytes
				}
			}
		case tui.RecordingCrashedMsg:
			status.State = tui.StateError.String()
			status.File = msg.File
		case tui.ClipVerifiedMsg:
			ok := msg.OK
			for i := range sessionClips {
				if sessionClips[i].File == msg.File {
					sessionClips[i].Verified = &ok
					sessionClips[i].VerifyErr = msg.Errors
				}
			}
		}
		statusMu.Unlock()

		p.Send(msg)
	}

	listener, err := listenTUICOSC(cfg.OSC.Port, func(message tuiOSCMessage) {
		sendToUI(tui.OSCReceivedMsg{
			Address: message.Address,
			Args:    renderArgs(message.Arguments),
			Source:  message.Source,
			Time:    time.Now(),
		})
		select {
		case oscCh <- message:
		default:
		}
	})
	if err != nil {
		return err
	}
	defer listener.Close()

	poller := sigpoll.New(mode.Name())
	poller.Start(deviceInfo.VideoDisplay, ffmpegPath, cfg.Device.FormatCode, func(msg tui.SignalStateMsg) {
		sendToUI(msg)
	})
	defer poller.Stop()

	var diskMonitor diskmon.Monitor
	diskMonitor.Start(outDir, func(msg tui.DiskStatMsg) {
		sendToUI(msg)
	})
	defer diskMonitor.Stop()

	rec := recorder.New(ffmpegPath, platform.Current())
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	defer cancelRunner()

	var healthServer health.Server
	if cfg.HTTP.Port > 0 {
		healthServer.SetClipsFunc(func() []health.ClipInfo {
			statusMu.Lock()
			defer statusMu.Unlock()
			clips := make([]health.ClipInfo, len(sessionClips))
			copy(clips, sessionClips)
			return clips
		})
		if err := healthServer.Start(cfg.HTTP.Port, cfg.HTTP.Bind, func() health.StatusSnapshot {
			statusMu.Lock()
			defer statusMu.Unlock()
			return status
		}); err != nil {
			return err
		}
		defer healthServer.Stop()
	}

	runnerDone := make(chan struct{})
	go func() {
		defer close(runnerDone)

		var (
			recordingFile    string
			recordingPath    string
			recordingStarted time.Time
			stopSizePoller   context.CancelFunc
		)

		cancelSizePoller := func() {
			if stopSizePoller != nil {
				stopSizePoller()
				stopSizePoller = nil
			}
		}

		startRecording := func() {
			if rec.IsRecording() {
				return
			}

			poller.Suspend()
			filename, err := rec.Start(mode, cfg.Recording.Profile, deviceInfo.VideoConfigValue, deviceInfo.AudioConfigValue, cfg.Recording.Prefix, outDir, verbose)
			if err != nil {
				poller.Resume()
				sendToUI(tui.ErrorBannerMsg{Text: err.Error()})
				return
			}

			recordingStarted = time.Now()
			recordingFile = filename
			recordingPath = filepath.Join(outDir, filename)
			cancelSizePoller()
			stopSizePoller = startFileSizePoller(runnerCtx, recordingFile, recordingPath, func(msg tui.FileSizeMsg) {
				sendToUI(msg)
			})
			sendToUI(tui.RecordingStartedMsg{
				File:   filename,
				Device: deviceInfo.VideoDisplay,
				Time:   recordingStarted,
			})
		}

		stopRecording := func() {
			if !rec.IsRecording() {
				return
			}

			exit, err := rec.StopAndWait(context.Background())
			cancelSizePoller()
			poller.Resume()
			if err != nil {
				sendToUI(tui.ErrorBannerMsg{Text: err.Error()})
				return
			}

			sizeBytes := fileSize(recordingPath)
			if exit.Path != "" {
				sizeBytes = fileSize(exit.Path)
			}

			duration := time.Since(recordingStarted)
			sendToUI(tui.RecordingStoppedMsg{
				File:      exit.Filename,
				Device:    deviceInfo.VideoDisplay,
				Duration:  duration,
				SizeBytes: sizeBytes,
			})
			clipVerifier.Verify(exit.Path, duration, mode.Name() == capture.ModeDecklink, func(msg tui.ClipVerifiedMsg) {
				msg.File = exit.Filename
				sendToUI(msg)
			})

			recordingFile = ""
			recordingPath = ""
			recordingStarted = time.Time{}
		}

		for {
			select {
			case <-runnerCtx.Done():
				cancelSizePoller()
				return
			case message := <-oscCh:
				switch {
				case cfg.OSC.RecordAddress != "" && message.Address == cfg.OSC.RecordAddress:
					startRecording()
				case cfg.OSC.StopAddress != "" && message.Address == cfg.OSC.StopAddress:
					stopRecording()
				}
			case cmd := <-commandCh:
				switch cmd {
				case tui.UserCmdRecord:
					startRecording()
				case tui.UserCmdStop:
					stopRecording()
				}
			case exit := <-rec.UnexpectedExit():
				cancelSizePoller()
				poller.Resume()
				recordingFile = ""
				recordingPath = ""
				recordingStarted = time.Time{}
				sendToUI(tui.RecordingCrashedMsg{
					File:        exit.Filename,
					Device:      deviceInfo.VideoDisplay,
					Err:         fmt.Errorf("ffmpeg exited unexpectedly (code %d)", exit.Code),
					Recoverable: false,
				})
			}
		}
	}()

	_, err = p.Run()
	cancelRunner()
	<-runnerDone
	if rec.IsRecording() {
		_, _ = rec.StopAndWait(context.Background())
	}
	return err
}

type tuiOSCMessage struct {
	Address   string
	Arguments []interface{}
	Source    string
}

type tuiOSCListener struct {
	conn net.PacketConn
	done chan struct{}
}

func listenTUICOSC(port int, handler func(tuiOSCMessage)) (*tuiOSCListener, error) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	listener := &tuiOSCListener{
		conn: conn,
		done: make(chan struct{}),
	}

	go listener.serve(handler)
	return listener, nil
}

func (l *tuiOSCListener) serve(handler func(tuiOSCMessage)) {
	defer close(l.done)

	buf := make([]byte, 65535)
	for {
		n, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			return
		}

		packet, err := goosc.ParsePacket(string(buf[:n]))
		if err != nil {
			continue
		}

		message, ok := packet.(*goosc.Message)
		if !ok || handler == nil {
			continue
		}

		handler(tuiOSCMessage{
			Address:   message.Address,
			Arguments: message.Arguments,
			Source:    addr.String(),
		})
	}
}

func (l *tuiOSCListener) Close() error {
	if l == nil || l.conn == nil {
		return nil
	}
	err := l.conn.Close()
	<-l.done
	return err
}

func renderArgs(args []interface{}) string {
	if len(args) == 0 {
		return ""
	}

	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, fmt.Sprint(arg))
	}
	return strings.Join(parts, " ")
}

func startFileSizePoller(ctx context.Context, file, path string, send func(tui.FileSizeMsg)) context.CancelFunc {
	pollCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				if send == nil {
					continue
				}
				send(tui.FileSizeMsg{
					File:      file,
					SizeBytes: fileSize(path),
				})
			}
		}
	}()
	return cancel
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// runPlaintext is the v0.1 plaintext path, extracted so runTUI can call it as fallback.
func runPlaintext(cfg cfgpkg.Config, ffmpegPath string, cmd *cobra.Command) error {
	if cfg.OSC.RecordAddress == "" {
		return errors.New("Error: No record trigger configured. Run 'osc-record capture record' first.")
	}
	if cfg.OSC.StopAddress == "" {
		return errors.New("Error: No stop trigger configured. Run 'osc-record capture stop' first.")
	}

	mode, modeWarning, err := capture.ResolveMode(cfg.Device.CaptureMode, ffmpegPath, runtime.GOOS, cfg.Device.FormatCode)
	if err != nil {
		return err
	}
	if modeWarning != "" {
		fmt.Println(modeWarning)
	}

	deviceInfo, updatedCfg, cfgChanged, err := ensureDevicesConfigured(ffmpegPath, mode, cfg, cmd.Flags().Changed("video-device"), cmd.Flags().Changed("audio-device"))
	if err != nil {
		return err
	}
	cfg = updatedCfg
	if cfgChanged {
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}

	if runtime.GOOS == "windows" && cfg.Recording.Profile == "prores" {
		fmt.Println("Warning: ProRes playback on Windows requires QuickTime or VLC.")
	}

	if mode.Name() == capture.ModeDecklink {
		if err := mode.SignalProbe(ffmpegPath, deviceInfo.VideoDisplay); err != nil {
			fmt.Printf("Warning: No valid signal detected on %q. Recording will fail until a signal is present.\n", deviceInfo.VideoDisplay)
		}
	}

	outDir := outputDir(cfg)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("Error: Output directory %s does not exist and could not be created: %v.", outDir, err)
	}

	triggerCh := make(chan oscpkg.Message, 16)
	listener, err := oscpkg.Listen(cfg.OSC.Port, func(message oscpkg.Message) {
		select {
		case triggerCh <- message:
		default:
		}
	})
	if err != nil {
		return err
	}
	defer listener.Close()

	rec := recorder.New(ffmpegPath, platform.Current())

	printRunSummary(cfg, mode, deviceInfo.VideoDisplay)
	fmt.Println()
	fmt.Println("Waiting for record trigger...")

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	for {
		select {
		case <-ctx.Done():
			if rec.IsRecording() {
				exit, err := rec.StopAndWait(context.Background())
				if err != nil {
					return err
				}
				if exit.Filename != "" {
					fmt.Printf("Recording saved: %s\n", exit.Filename)
				}
			}
			return nil
		case exit := <-rec.UnexpectedExit():
			fmt.Printf("Error: ffmpeg exited unexpectedly (code %d). Waiting for record trigger...\n", exit.Code)
		case message := <-triggerCh:
			verbosef("OSC %s %v", message.Address, message.Arguments)

			switch message.Address {
			case cfg.OSC.RecordAddress:
				if rec.IsRecording() {
					fmt.Println("Warning: Record trigger received but already recording. Ignoring.")
					continue
				}

				filename, err := rec.Start(mode, cfg.Recording.Profile, deviceInfo.VideoConfigValue, deviceInfo.AudioConfigValue, cfg.Recording.Prefix, outDir, verbose)
				if err != nil {
					return err
				}
				fmt.Printf("Recording started: %s\n", filename)
			case cfg.OSC.StopAddress:
				if !rec.IsRecording() {
					fmt.Println("Warning: Stop trigger received but not recording. Ignoring.")
					continue
				}

				exit, err := rec.StopAndWait(context.Background())
				if err != nil {
					return err
				}
				fmt.Printf("Recording saved: %s\n", exit.Filename)
				fmt.Println("Waiting for record trigger...")
			}
		}
	}
}

func printRunSummary(cfg cfgpkg.Config, mode capture.CaptureMode, deviceName string) {
	fmt.Println("osc-record running")
	fmt.Printf("  OSC port:    %d\n", cfg.OSC.Port)
	fmt.Printf("  Record:      %s\n", cfg.OSC.RecordAddress)
	fmt.Printf("  Stop:        %s\n", cfg.OSC.StopAddress)
	fmt.Printf("  Capture:     %s\n", mode.Summary())
	fmt.Printf("  Profile:     %s\n", cfg.Recording.Profile)
	fmt.Printf("  Prefix:      %s\n", cfg.Recording.Prefix)
	fmt.Printf("  Output:      %s\n", outputDir(cfg))
	fmt.Printf("  Device:      %s\n", deviceName)
}
