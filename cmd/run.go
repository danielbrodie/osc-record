package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	"github.com/danielbrodie/osc-record/internal/devices"
	"github.com/danielbrodie/osc-record/internal/diskmon"
	"github.com/danielbrodie/osc-record/internal/health"
	"github.com/danielbrodie/osc-record/internal/manifest"
	"github.com/danielbrodie/osc-record/internal/multirecorder"
	oscpkg "github.com/danielbrodie/osc-record/internal/osc"
	"github.com/danielbrodie/osc-record/internal/platform"
	"github.com/danielbrodie/osc-record/internal/preview"
	"github.com/danielbrodie/osc-record/internal/recorder"
	"github.com/danielbrodie/osc-record/internal/scanner"
	"github.com/danielbrodie/osc-record/internal/audiometer"
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

type resolvedDevice struct {
	Config   cfgpkg.DeviceConfig
	Mode     capture.CaptureMode
	Selected selectedDevices
}

type multiRecordingState struct {
	Device    multirecorder.DeviceInfo
	Path      string
	StartedAt time.Time
}

func applyRunFlagOverrides(cmd *cobra.Command, cfg cfgpkg.Config) (cfgpkg.Config, error) {
	if cfg.HasMultipleDevices() {
		if cmd.Flags().Changed("capture-mode") || cmd.Flags().Changed("video-device") || cmd.Flags().Changed("audio-device") {
			return cfg, errors.New("Error: --capture-mode, --video-device, and --audio-device are only supported with a single configured device. Edit [[devices]] in config instead.")
		}
	}

	devices := cfg.ActiveDevices()
	if len(devices) == 0 {
		devices = []cfgpkg.DeviceConfig{cfg.Device}
	}
	primary := devices[0]

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
		primary.CaptureMode = value
	}
	if cmd.Flags().Changed("video-device") {
		value, _ := cmd.Flags().GetString("video-device")
		primary.Name = value
	}
	if cmd.Flags().Changed("audio-device") {
		value, _ := cmd.Flags().GetString("audio-device")
		primary.Audio = value
	}
	devices[0] = primary
	cfg.SetDevices(devices, cfg.UsesDevicesArray() || len(devices) > 1)
	return cfg, nil
}

func ensureDeviceConfigured(ffmpegPath string, mode capture.CaptureMode, deviceCfg cfgpkg.DeviceConfig, videoOverride, audioOverride bool) (selectedDevices, cfgpkg.DeviceConfig, bool, error) {
	var changed bool

	group, err := devices.ProbeMode(ffmpegPath, mode.Name())
	if err != nil {
		return selectedDevices{}, deviceCfg, false, err
	}

	selected := selectedDevices{}
	if deviceCfg.Name == "" && !videoOverride {
		video, err := promptForDevice(group.Video, "capture device", mode.Name() == capture.ModeDecklink)
		if err != nil {
			return selectedDevices{}, deviceCfg, false, err
		}
		deviceCfg.Name = video.ConfigValue()
		selected.VideoDisplay = video.Name
		selected.VideoConfigValue = video.ConfigValue()
		changed = true
	} else {
		video, err := devices.MatchDevice(group.Video, deviceCfg.Name)
		if err != nil {
			return selectedDevices{}, deviceCfg, false, fmt.Errorf("Error: Video device %q not found. Run 'osc-record devices' to list available devices.", deviceCfg.Name)
		}
		selected.VideoDisplay = video.Name
		selected.VideoConfigValue = deviceCfg.Name
	}

	if mode.NeedsAudio() {
		if deviceCfg.Audio == "" && !audioOverride {
			// Try to auto-match audio device by video device name before prompting.
			if matched, err := devices.BestAudioMatch(group.Audio, selected.VideoDisplay); err == nil {
				deviceCfg.Audio = matched.ConfigValue()
				selected.AudioDisplay = matched.Name
				selected.AudioConfigValue = matched.ConfigValue()
				changed = true
			} else {
				audio, err := promptForDevice(group.Audio, "audio device", false)
				if err != nil {
					return selectedDevices{}, deviceCfg, false, err
				}
				deviceCfg.Audio = audio.ConfigValue()
				selected.AudioDisplay = audio.Name
				selected.AudioConfigValue = audio.ConfigValue()
				changed = true
			}
		} else {
			audio, err := devices.MatchDevice(group.Audio, deviceCfg.Audio)
			if err != nil {
				return selectedDevices{}, deviceCfg, false, fmt.Errorf("Error: Audio device %q not found. Run 'osc-record devices' to list available devices.", deviceCfg.Audio)
			}
			selected.AudioDisplay = audio.Name
			selected.AudioConfigValue = deviceCfg.Audio
		}
	}

	return selected, deviceCfg, changed, nil
}

func resolveConfiguredDevices(ffmpegPath string, cfg cfgpkg.Config, videoOverride, audioOverride bool) ([]resolvedDevice, cfgpkg.Config, []string, bool, error) {
	configured := cfg.ActiveDevices()
	if len(configured) == 0 {
		configured = []cfgpkg.DeviceConfig{cfg.Device}
	}

	resolved := make([]resolvedDevice, 0, len(configured))
	warnings := make([]string, 0, len(configured))
	changed := false

	for i, deviceCfg := range configured {
		mode, warning, err := capture.ResolveMode(deviceCfg.CaptureMode, ffmpegPath, runtime.GOOS, deviceCfg.FormatCode, deviceCfg.VideoInput)
		if err != nil {
			return nil, cfg, nil, false, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}

		selected, updatedCfg, cfgChanged, err := ensureDeviceConfigured(ffmpegPath, mode, deviceCfg, videoOverride && i == 0, audioOverride && i == 0)
		if err != nil {
			return nil, cfg, nil, false, err
		}

		configured[i] = updatedCfg
		changed = changed || cfgChanged
		resolved = append(resolved, resolvedDevice{
			Config:   updatedCfg,
			Mode:     mode,
			Selected: selected,
		})
	}

	cfg.SetDevices(configured, cfg.UsesDevicesArray() || len(configured) > 1)
	return resolved, cfg, warnings, changed, nil
}

func configuredDeviceNames(devices []resolvedDevice) []string {
	names := make([]string, 0, len(devices))
	for _, device := range devices {
		names = append(names, device.Selected.VideoDisplay)
	}
	return names
}

func primaryDevice(devices []resolvedDevice) resolvedDevice {
	return devices[0]
}

// needsAutoDetect returns true if the primary device is decklink, the config
// is single-device, and either video_input or format_code is missing/auto.
func needsAutoDetect(cfg cfgpkg.Config, primary resolvedDevice) bool {
	if primary.Mode.Name() != capture.ModeDecklink {
		return false
	}
	if cfg.HasMultipleDevices() {
		return false
	}
	vi := primary.Config.VideoInput
	fc := primary.Config.FormatCode
	inputMissing := vi == "" || vi == "auto"
	formatMissing := fc == ""
	return inputMissing || formatMissing
}

func startupProbeWarnings(ffmpegPath string, devices []resolvedDevice) []string {
	// Always use goroutine+timeout regardless of device count to avoid hanging startup.
	const probeTimeout = 5 * time.Second
	if len(devices) <= 1 {
		warnings := make([]string, 0, len(devices))
		for _, device := range devices {
			if device.Mode.Name() != capture.ModeDecklink {
				continue
			}
			probeDone := make(chan error, 1)
			go func() { probeDone <- device.Mode.SignalProbe(ffmpegPath, device.Selected.VideoDisplay) }()
			select {
			case err := <-probeDone:
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("Warning: No valid signal detected on %q. Recording will fail until a signal is present.", device.Selected.VideoDisplay))
				}
			case <-time.After(probeTimeout):
				warnings = append(warnings, fmt.Sprintf("Warning: Signal probe timed out on %q. Check device connection.", device.Selected.VideoDisplay))
			}
		}
		return warnings
	}

	warnings := make([]string, 0, len(devices))
	results := make([]string, len(devices))

	var wg sync.WaitGroup
	for i, device := range devices {
		if device.Mode.Name() != capture.ModeDecklink {
			continue
		}

		wg.Add(1)
		go func(index int, device resolvedDevice) {
			defer wg.Done()

			probeDone := make(chan error, 1)
			go func() {
				probeDone <- device.Mode.SignalProbe(ffmpegPath, device.Selected.VideoDisplay)
			}()

			select {
			case err := <-probeDone:
				if err != nil {
					results[index] = fmt.Sprintf("Warning: No valid signal detected on %q. Recording will fail until a signal is present.", device.Selected.VideoDisplay)
				}
			case <-time.After(5 * time.Second):
				results[index] = fmt.Sprintf("Warning: Signal probe timed out on %q after 5s. Recording may fail until a signal is present.", device.Selected.VideoDisplay)
			}
		}(i, device)
	}
	wg.Wait()

	for _, warning := range results {
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func toMultiRecorderDevices(devices []resolvedDevice) []multirecorder.DeviceInfo {
	result := make([]multirecorder.DeviceInfo, 0, len(devices))
	for _, device := range devices {
		result = append(result, multirecorder.DeviceInfo{
			Name:        device.Selected.VideoDisplay,
			Mode:        device.Mode,
			VideoDevice: device.Selected.VideoConfigValue,
			AudioDevice: device.Selected.AudioConfigValue,
		})
	}
	return result
}

func toStatusDevices(devices []resolvedDevice) []tui.DeviceStatus {
	result := make([]tui.DeviceStatus, 0, len(devices))
	for _, device := range devices {
		result = append(result, tui.DeviceStatus{
			Device:      device.Selected.VideoDisplay,
			CaptureMode: device.Mode.Name(),
			FormatCode:  device.Config.FormatCode,
			State:       tui.StateIdle,
		})
	}
	return result
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
	// Apply defaults for any unconfigured OSC addresses (e.g. after a failed setup).
	defaults := cfgpkg.Defaults()
	if cfg.OSC.RecordAddress == "" {
		cfg.OSC.RecordAddress = defaults.OSC.RecordAddress
	}
	if cfg.OSC.StopAddress == "" {
		cfg.OSC.StopAddress = defaults.OSC.StopAddress
	}

	resolvedDevices, updatedCfg, modeWarnings, cfgChanged, err := resolveConfiguredDevices(ffmpegPath, cfg, cmd.Flags().Changed("video-device"), cmd.Flags().Changed("audio-device"))
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

	primary := primaryDevice(resolvedDevices)
	model := tui.New(cfg.OSC.RecordAddress, cfg.OSC.StopAddress, configuredDeviceNames(resolvedDevices))
	model.SetStatusDevices(toStatusDevices(resolvedDevices))
	model.SetChecklistConfig(tui.ChecklistConfig{
		FFmpegPath:    ffmpegPath,
		DeviceName:    primary.Selected.VideoDisplay,
		FormatCode:    primary.Config.FormatCode,
		OutputDir:     outDir,
		CaptureMode:   primary.Mode.Name(),
		RecordAddress: cfg.OSC.RecordAddress,
		StopAddress:   cfg.OSC.StopAddress,
	})
	model.SetSlate(tui.Slate{
		Show:  cfg.Recording.Show,
		Scene: cfg.Recording.Scene,
		Take:  cfg.Recording.Take,
	})
	commandCh := model.Commands()
	slateCh := model.SlateChanges()

	// Channel for InputChoiceOverlay to forward user selection to auto-detect goroutine.
	inputChoiceCh := make(chan string, 1)
	model.SetInputChoiceCh(inputChoiceCh)
	p := tea.NewProgram(model, tea.WithAltScreen())
	oscCh := make(chan tuiOSCMessage, 32)
	clipVerifier := verifier.Verifier{}

	var (
		statusMu sync.Mutex
		status   = health.StatusSnapshot{
			State:         tui.StateIdle.String(),
			Device:        primary.Selected.VideoDisplay,
			Format:        primary.Config.FormatCode,
			OSCPort:       cfg.OSC.Port,
			RecordAddress: cfg.OSC.RecordAddress,
			StopAddress:   cfg.OSC.StopAddress,
		}
		sessionClips []health.ClipInfo
	)

	// uiStarted is closed once p.Run() has been called, allowing goroutines
	// that want to p.Send() to wait until the event loop is ready.
	uiStarted := make(chan struct{})

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

		// Send in a goroutine so callers never block waiting for the event loop.
		go func() {
			<-uiStarted
			p.Send(msg)
		}()
	}

	logWarning := func(text string) {
		sendToUI(tui.LogMsg{Time: time.Now(), Text: text})
		sendToUI(tui.ErrorBannerMsg{Text: text})
	}

	for _, warning := range modeWarnings {
		logWarning(warning)
	}
	fmt.Print("Starting osc-record")
	for i, d := range resolvedDevices {
		if i == 0 {
			fmt.Printf(" (%s", d.Selected.VideoDisplay)
		} else {
			fmt.Printf(", %s", d.Selected.VideoDisplay)
		}
	}
	fmt.Print(")... ")
	for _, warning := range startupProbeWarnings(ffmpegPath, resolvedDevices) {
		logWarning(warning)
	}
	fmt.Println("ready.")

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

	poller := sigpoll.New(primary.Mode.Name())
	poller.Start(primary.Selected.VideoDisplay, ffmpegPath, primary.Config.FormatCode, primary.Config.VideoInput, func(msg tui.SignalStateMsg) {
		sendToUI(msg)
	})
	defer poller.Stop()

	// Audio meter — persistent ffmpeg process reading RMS levels during idle.
	// Only runs on decklink devices (avfoundation has no exclusive lock issue, but
	// the audio meter still needs the device so we keep the same suspend/resume pattern).
	// Start is delayed until after the initial signal probe completes (~3s).
	var aMeter audiometer.Meter
	var audioMu sync.Mutex
	audioInputArgs := primary.Mode.BuildInputArgs(primary.Selected.VideoConfigValue, primary.Selected.AudioConfigValue)
	startAudioMeter := func() {
		audioMu.Lock()
		args := audioInputArgs
		audioMu.Unlock()
		aMeter.Start(ffmpegPath, args, func(msg tui.AudioLevelMsg) {
			sendToUI(msg)
		})
	}
	stopAudioMeter := func() { aMeter.Stop() }
	defer stopAudioMeter()

	// The signal poller and audio meter both need exclusive device access.
	// Register hooks so the poller stops the meter before probing and restarts it after.
	poller.SetProbeHooks(stopAudioMeter, startAudioMeter)

	// detectDoneCh is closed when the auto-detect goroutine finishes (success,
	// failure, or cancellation). The runner goroutine waits on this before
	// starting recording or grabbing a preview so it never contends for the
	// DeckLink device while detection is in progress.
	detectDoneCh := make(chan struct{})
	var cancelDetectFn context.CancelFunc // set if auto-detect is running

	runAutoDetect := needsAutoDetect(cfg, primary)
	if runAutoDetect {
		// Signal the TUI that we're probing.
		sendToUI(tui.SignalStateMsg{Device: primary.Selected.VideoDisplay, Probing: true})
	}

	if runAutoDetect {
		go func() {
			defer close(detectDoneCh)

			// Suspend poller and ensure audio meter is stopped during detection.
			poller.Suspend()
			stopAudioMeter()

			detectCtx, cancelDetect := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancelDetect()
			cancelDetectFn = cancelDetect

			result, err := scanner.AutoDetect(detectCtx, ffmpegPath, primary.Selected.VideoDisplay, func(msg tui.AutoDetectProgressMsg) {
				sendToUI(msg)
			})

			if err != nil {
				sendToUI(tui.AutoDetectCompleteMsg{Err: err})
				// Clear probing state and resume normal operation.
				sendToUI(tui.SignalStateMsg{Device: primary.Selected.VideoDisplay, Probing: false})
				poller.Resume()
				startAudioMeter()
				return
			}

			// If both inputs locked, ask user to disambiguate.
			if result.BothLocked {
				sendToUI(tui.AutoDetectCompleteMsg{BothLocked: true})

				// Wait for user to pick an input, or for cancellation (e.g.
				// record trigger called waitForDetect → cancelDetectFn).
				var chosenInput string
				select {
				case chosenInput = <-inputChoiceCh:
				case <-detectCtx.Done():
					sendToUI(tui.LogMsg{Time: time.Now(), Text: "Auto-detect cancelled (record triggered)"})
					sendToUI(tui.SignalStateMsg{Device: primary.Selected.VideoDisplay, Probing: false})
					poller.Resume()
					startAudioMeter()
					return
				}
				if chosenInput == "" {
					// User pressed Esc — abort auto-detect.
					sendToUI(tui.LogMsg{Time: time.Now(), Text: "Auto-detect cancelled"})
					sendToUI(tui.SignalStateMsg{Device: primary.Selected.VideoDisplay, Probing: false})
					poller.Resume()
					startAudioMeter()
					return
				}

				// Now scan format codes for the chosen input.
				result, err = scanner.AutoDetectFormat(detectCtx, ffmpegPath, primary.Selected.VideoDisplay, chosenInput, func(msg tui.AutoDetectProgressMsg) {
					sendToUI(msg)
				})
				if err != nil {
					sendToUI(tui.AutoDetectCompleteMsg{Err: err})
					sendToUI(tui.SignalStateMsg{Device: primary.Selected.VideoDisplay, Probing: false})
					poller.Resume()
					startAudioMeter()
					return
				}
			}

			// Update config with discovered values.
			devices := cfg.ActiveDevices()
			devices[0].VideoInput = result.VideoInput
			devices[0].FormatCode = result.FormatCode
			cfg.SetDevices(devices, cfg.UsesDevicesArray())
			primary.Config = devices[0]
			if saveErr := saveConfig(cfg); saveErr != nil {
				sendToUI(tui.ErrorBannerMsg{Text: "Auto-detect: failed to save config: " + saveErr.Error()})
			}

			// Update the live capture mode so recording, preview, and
			// metering all use the newly-detected input/format.
			primary.Mode = capture.DecklinkMode{
				FormatCode: result.FormatCode,
				VideoInput: result.VideoInput,
			}

			// Rebuild audioInputArgs with the updated mode.
			audioMu.Lock()
			audioInputArgs = primary.Mode.BuildInputArgs(primary.Selected.VideoConfigValue, primary.Selected.AudioConfigValue)
			audioMu.Unlock()

			sendToUI(tui.AutoDetectCompleteMsg{
				VideoInput: result.VideoInput,
				FormatCode: result.FormatCode,
				FormatDesc: result.FormatDesc,
			})
			sendToUI(tui.ConfigUpdatedMsg{
				VideoInput: result.VideoInput,
				FormatCode: result.FormatCode,
			})

			// Restart poller with discovered values.
			poller.Stop()
			poller = sigpoll.New(primary.Mode.Name())
			poller.Start(primary.Selected.VideoDisplay, ffmpegPath, result.FormatCode, result.VideoInput, func(msg tui.SignalStateMsg) {
				sendToUI(msg)
			})
			poller.SetProbeHooks(stopAudioMeter, startAudioMeter)
			startAudioMeter()
		}()
	} else {
		// No auto-detect — mark detection as done immediately.
		close(detectDoneCh)
		// Start audio meter on the normal 4s delay.
		go func() {
			time.Sleep(4 * time.Second)
			startAudioMeter()
		}()
	}

	// waitForDetect cancels any running auto-detect and blocks until the
	// goroutine has released the device. Safe to call when detect is already
	// done (detectDoneCh is already closed).
	waitForDetect := func() {
		if cancelDetectFn != nil {
			cancelDetectFn()
		}
		<-detectDoneCh
	}

	useMultiRecorder := len(resolvedDevices) > 1
	multiDevices := toMultiRecorderDevices(resolvedDevices)

	var diskMonitor diskmon.Monitor
	diskMonitor.Start(outDir, func(msg tui.DiskStatMsg) {
		sendToUI(msg)
	})
	defer diskMonitor.Stop()

	singleRecorder := recorder.New(ffmpegPath, platform.Current())
	multiRecorder := multirecorder.New(ffmpegPath, platform.Current(), multiDevices)
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
			multiRecordings  = map[string]multiRecordingState{}
			recordingPaths   = map[string]string{}
			sizePollers      = map[string]context.CancelFunc{}
			completedClips   []string
			currentViewPath  string
			currentSlate     = recorder.Slate{
				Show:  cfg.Recording.Show,
				Scene: cfg.Recording.Scene,
				Take:  cfg.Recording.Take,
			}
		)

		cancelSizePoller := func() {
			if stopSizePoller != nil {
				stopSizePoller()
				stopSizePoller = nil
			}
		}

		cancelMultiSizePoller := func(filename string) {
			if cancel, ok := sizePollers[filename]; ok {
				cancel()
				delete(sizePollers, filename)
			}
		}

		cancelAllMultiSizePollers := func() {
			for deviceName := range sizePollers {
				cancelMultiSizePoller(deviceName)
			}
		}

		refreshCurrentViewPath := func() {
			currentViewPath = ""
			for _, device := range resolvedDevices {
				if path := recordingPaths[device.Selected.VideoDisplay]; path != "" {
					currentViewPath = path
					return
				}
			}
		}

		startRecording := func() {
			// If auto-detect is still running, cancel it and wait for the
			// device to be released before we try to open it for recording.
			waitForDetect()

			if useMultiRecorder {
				if multiRecorder.IsRecording() {
					return
				}

				poller.Suspend(); stopAudioMeter()
				startedAt := time.Now()
				filenames, err := multiRecorder.Start(primary.Mode, cfg.Recording.Profile, cfg.Recording.Prefix, outDir, currentSlate, verbose)
				if err != nil {
					poller.Resume(); startAudioMeter()
					sendToUI(tui.ErrorBannerMsg{Text: err.Error()})
					return
				}

				cancelAllMultiSizePollers()
				multiRecordings = make(map[string]multiRecordingState, len(filenames))
				for i, filename := range filenames {
					device := multiDevices[i]
					path := filepath.Join(outDir, filename)
					multiRecordings[filename] = multiRecordingState{
						Device:    device,
						Path:      path,
						StartedAt: startedAt,
					}
					recordingPaths[device.Name] = path
					sizePollers[filename] = startFileSizePoller(runnerCtx, filename, path, func(msg tui.FileSizeMsg) {
						sendToUI(msg)
					})
					currentViewPath = path
					sendToUI(tui.RecordingStartedMsg{
						File:   filename,
						Device: device.Name,
						Time:   startedAt,
					})
				}
				refreshCurrentViewPath()
				return
			}

			if singleRecorder.IsRecording() {
				return
			}

			poller.Suspend(); stopAudioMeter()
			filename, err := singleRecorder.Start(primary.Mode, cfg.Recording.Profile, primary.Selected.VideoConfigValue, primary.Selected.AudioConfigValue, cfg.Recording.Prefix, outDir, currentSlate, verbose)
			if err != nil {
				poller.Resume(); startAudioMeter()
				sendToUI(tui.ErrorBannerMsg{Text: err.Error()})
				return
			}

			recordingStarted = time.Now()
			recordingFile = filename
			recordingPath = filepath.Join(outDir, filename)
			currentViewPath = recordingPath
			cancelSizePoller()
			stopSizePoller = startFileSizePoller(runnerCtx, recordingFile, recordingPath, func(msg tui.FileSizeMsg) {
				sendToUI(msg)
			})
			sendToUI(tui.RecordingStartedMsg{
				File:   filename,
				Device: primary.Selected.VideoDisplay,
				Time:   recordingStarted,
			})
		}

		stopRecording := func() {
			if useMultiRecorder {
				if !multiRecorder.IsRecording() {
					return
				}

				stopped := make(map[string]multiRecordingState, len(multiRecordings))
				for filename, state := range multiRecordings {
					stopped[filename] = state
				}

				results := multiRecorder.Stop()
				cancelAllMultiSizePollers()
				poller.Resume(); startAudioMeter()
				for _, device := range resolvedDevices {
					var (
						filename string
						state    multiRecordingState
						ok       bool
					)
					for file, current := range stopped {
						if current.Device.Name == device.Selected.VideoDisplay {
							filename = file
							state = current
							ok = true
							break
						}
					}
					if !ok {
						continue
					}

					delete(multiRecordings, filename)
					delete(recordingPaths, state.Device.Name)
					duration := time.Since(state.StartedAt)
					sizeBytes := fileSize(state.Path)
					sendToUI(tui.RecordingStoppedMsg{
						File:      filename,
						Device:    state.Device.Name,
						Duration:  duration,
						SizeBytes: sizeBytes,
					})
					clipVerifier.Verify(state.Path, duration, state.Device.Mode.Name() == capture.ModeDecklink, func(msg tui.ClipVerifiedMsg) {
						msg.File = filename
						sendToUI(msg)
					})
					completedClips = append(completedClips, filename)
					if err := results[filename]; err != nil {
						sendToUI(tui.ErrorBannerMsg{Text: err.Error()})
					}
				}
				refreshCurrentViewPath()
				return
			}

			if !singleRecorder.IsRecording() {
				return
			}

			exit, err := singleRecorder.StopAndWait(context.Background())
			cancelSizePoller()
			poller.Resume(); startAudioMeter()
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
				Device:    primary.Selected.VideoDisplay,
				Duration:  duration,
				SizeBytes: sizeBytes,
			})
			clipVerifier.Verify(exit.Path, duration, primary.Mode.Name() == capture.ModeDecklink, func(msg tui.ClipVerifiedMsg) {
				msg.File = exit.Filename
				sendToUI(msg)
			})

			completedClips = append(completedClips, exit.Filename)
			recordingFile = ""
			recordingPath = ""
			recordingStarted = time.Time{}
			currentViewPath = ""
		}

		for {
			select {
			case <-runnerCtx.Done():
				cancelSizePoller()
				cancelAllMultiSizePollers()
				return
			case message := <-oscCh:
				// Always forward to the TUI so the wizard overlay can capture it.
				sendToUI(tui.WizardOSCSeenMsg{Address: message.Address})
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
				case tui.UserCmdGrabPreview:
					go func() {
						waitForDetect()
						poller.Suspend(); stopAudioMeter()
						inputArgs := primary.Mode.BuildInputArgs(primary.Selected.VideoConfigValue, primary.Selected.AudioConfigValue)
						path, err := preview.GrabFrame(ffmpegPath, inputArgs, primary.Selected.VideoDisplay)
						poller.Resume(); startAudioMeter()
						sendToUI(tui.PreviewGrabbedMsg{Path: path, Err: err})
					}()
				case tui.UserCmdViewClip:
					go func() {
						if currentViewPath != "" {
							_ = openPath(currentViewPath)
						} else if len(completedClips) > 0 {
							_ = openPath(filepath.Join(outDir, completedClips[len(completedClips)-1]))
						}
					}()
				case tui.UserCmdTakeReset:
					currentSlate.Take = "1"
					cfg.Recording.Take = "1"
				case tui.UserCmdScan:
					go func() {
						poller.Suspend(); stopAudioMeter()
						scanCtx, cancelScan := context.WithCancel(runnerCtx)
						_ = cancelScan
						results := scanner.Run(scanCtx, ffmpegPath, primary.Selected.VideoDisplay, primary.Config.VideoInput, func(msg tui.ScanProgressMsg) {
							sendToUI(msg)
						})
						poller.Resume(); startAudioMeter()
						sendToUI(tui.ScanCompleteMsg{Results: results})
					}()
				}
			case slate := <-slateCh:
				currentSlate = recorder.Slate{
					Show:  slate.Show,
					Scene: slate.Scene,
					Take:  slate.Take,
				}
				cfg.Recording.Show = slate.Show
				cfg.Recording.Scene = slate.Scene
				cfg.Recording.Take = slate.Take
			case exit := <-singleRecorder.UnexpectedExit():
				if useMultiRecorder {
					continue
				}
				cancelSizePoller()
				poller.Resume(); startAudioMeter()
				recordingFile = ""
				recordingPath = ""
				recordingStarted = time.Time{}
				currentViewPath = ""
				sendToUI(tui.RecordingCrashedMsg{
					File:        exit.Filename,
					Device:      primary.Selected.VideoDisplay,
					Err:         fmt.Errorf("ffmpeg exited unexpectedly (code %d)", exit.Code),
					Recoverable: false,
				})
			case exit := <-multiRecorder.UnexpectedExits():
				if !useMultiRecorder {
					continue
				}
				recording, ok := multiRecordings[exit.Filename]
				cancelMultiSizePoller(exit.Filename)
				delete(multiRecordings, exit.Filename)
				if ok {
					delete(recordingPaths, recording.Device.Name)
				}
				if ok && recording.Device.Name == primary.Selected.VideoDisplay {
					poller.Resume(); startAudioMeter()
				}
				refreshCurrentViewPath()
				deviceName := primary.Selected.VideoDisplay
				if ok {
					deviceName = recording.Device.Name
				}
				sendToUI(tui.RecordingCrashedMsg{
					File:        exit.Filename,
					Device:      deviceName,
					Err:         fmt.Errorf("ffmpeg exited unexpectedly (code %d)", exit.Code),
					Recoverable: false,
				})
			}
		}
	}()

	go func() { close(uiStarted) }()
	_, err = p.Run()
	cancelRunner()
	<-runnerDone
	if useMultiRecorder {
		if multiRecorder.IsRecording() {
			_ = multiRecorder.Stop()
		}
	} else {
		if singleRecorder.IsRecording() {
			_, _ = singleRecorder.StopAndWait(context.Background())
		}
	}
	statusMu.Lock()
	clipsForManifest := make([]tui.ClipInfo, len(sessionClips))
	copy(clipsForManifest, sessionClips)
	statusMu.Unlock()
	if len(clipsForManifest) > 0 {
		if writeErr := manifest.Write(clipsForManifest, cfg, outDir); writeErr != nil && err == nil {
			err = writeErr
		}
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
	addr := fmt.Sprintf(":%d", port)
	lc := reusePortListenConfig()
	conn, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		conn, err = net.ListenPacket("udp", addr)
	}
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			return nil, fmt.Errorf("Error: Could not bind to port %d: address already in use.\nKill any existing osc-record process (pkill -f osc-record) or use --port to specify a different port.", port)
		}
		return nil, fmt.Errorf("Error: Could not bind to port %d: %v", port, err)
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

		if handler == nil {
			continue
		}

		// Collect messages from either a plain Message or a Bundle (e.g. Disguise sends bundles).
		var messages []*goosc.Message
		switch p := packet.(type) {
		case *goosc.Message:
			messages = []*goosc.Message{p}
		case *goosc.Bundle:
			messages = p.Messages
		}

		for _, message := range messages {
			handler(tuiOSCMessage{
				Address:   message.Address,
				Arguments: message.Arguments,
				Source:    addr.String(),
			})
		}
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
	defaults := cfgpkg.Defaults()
	if cfg.OSC.RecordAddress == "" {
		cfg.OSC.RecordAddress = defaults.OSC.RecordAddress
		fmt.Printf("Note: No record address configured — using default %s\n", cfg.OSC.RecordAddress)
	}
	if cfg.OSC.StopAddress == "" {
		cfg.OSC.StopAddress = defaults.OSC.StopAddress
		fmt.Printf("Note: No stop address configured — using default %s\n", cfg.OSC.StopAddress)
	}

	resolvedDevices, updatedCfg, modeWarnings, cfgChanged, err := resolveConfiguredDevices(ffmpegPath, cfg, cmd.Flags().Changed("video-device"), cmd.Flags().Changed("audio-device"))
	if err != nil {
		return err
	}
	cfg = updatedCfg
	for _, warning := range modeWarnings {
		fmt.Println(warning)
	}
	if cfgChanged {
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}

	if runtime.GOOS == "windows" && cfg.Recording.Profile == "prores" {
		fmt.Println("Warning: ProRes playback on Windows requires QuickTime or VLC.")
	}

	for _, warning := range startupProbeWarnings(ffmpegPath, resolvedDevices) {
		fmt.Println(warning)
	}

	primary := primaryDevice(resolvedDevices)

	// Plaintext auto-detect: blocking probe + numbered prompt for disambiguation.
	if needsAutoDetect(cfg, primary) {
		fmt.Println("Probing video inputs...")

		detectCtx, cancelDetect := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancelDetect()
		result, detectErr := scanner.AutoDetect(detectCtx, ffmpegPath, primary.Selected.VideoDisplay, func(msg tui.AutoDetectProgressMsg) {
			fmt.Printf("  %s\n", msg.Detail)
		})

		if detectErr != nil {
			fmt.Printf("Auto-detect failed: %v\n", detectErr)
		} else if result.BothLocked {
			// Disambiguation prompt.
			fmt.Println("\nBoth HDMI and SDI have a live signal.")
			fmt.Println("  1. HDMI")
			fmt.Println("  2. SDI")
			fmt.Printf("Choose [1]: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)

			chosenInput := "hdmi"
			if line == "2" || strings.EqualFold(line, "sdi") {
				chosenInput = "sdi"
			}

			result, detectErr = scanner.AutoDetectFormat(detectCtx, ffmpegPath, primary.Selected.VideoDisplay, chosenInput, func(msg tui.AutoDetectProgressMsg) {
				fmt.Printf("  %s\n", msg.Detail)
			})
			if detectErr != nil {
				fmt.Printf("Format scan failed: %v\n", detectErr)
			}
		}

		if detectErr == nil && result != nil && result.VideoInput != "" {
			fmt.Printf("✓ Auto-detected: %s %s (%s)\n\n", result.VideoInput, result.FormatDesc, result.FormatCode)

			devices := cfg.ActiveDevices()
			devices[0].VideoInput = result.VideoInput
			devices[0].FormatCode = result.FormatCode
			cfg.SetDevices(devices, cfg.UsesDevicesArray())
			primary.Config = devices[0]
			primary.Mode = capture.DecklinkMode{
				FormatCode: result.FormatCode,
				VideoInput: result.VideoInput,
			}
			if saveErr := saveConfig(cfg); saveErr != nil {
				fmt.Printf("Warning: failed to save config: %v\n", saveErr)
			} else {
				fmt.Println("Config saved.")
			}
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

	useMultiRecorder := len(resolvedDevices) > 1
	multiDevices := toMultiRecorderDevices(resolvedDevices)
	singleRecorder := recorder.New(ffmpegPath, platform.Current())
	multiRecorder := multirecorder.New(ffmpegPath, platform.Current(), multiDevices)
	activeMultiRecordings := map[string]multirecorder.DeviceInfo{}

	if useMultiRecorder {
		printMultiRunSummary(cfg, resolvedDevices)
	} else {
		printRunSummary(cfg, primary.Mode, primary.Selected.VideoDisplay)
	}
	fmt.Println()
	fmt.Println("Waiting for record trigger...")

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	for {
		select {
		case <-ctx.Done():
			if useMultiRecorder {
				if multiRecorder.IsRecording() {
					results := multiRecorder.Stop()
					for _, device := range multiDevices {
						for filename, info := range activeMultiRecordings {
							if info.Name != device.Name {
								continue
							}
							if err := results[filename]; err != nil {
								return err
							}
							fmt.Printf("Recording saved: %s\n", filename)
							delete(activeMultiRecordings, filename)
							break
						}
					}
				}
				return nil
			}

			if singleRecorder.IsRecording() {
				exit, err := singleRecorder.StopAndWait(context.Background())
				if err != nil {
					return err
				}
				if exit.Filename != "" {
					fmt.Printf("Recording saved: %s\n", exit.Filename)
				}
			}
			return nil
		case exit := <-singleRecorder.UnexpectedExit():
			if useMultiRecorder {
				continue
			}
			fmt.Printf("Error: ffmpeg exited unexpectedly (code %d). Waiting for record trigger...\n", exit.Code)
		case exit := <-multiRecorder.UnexpectedExits():
			if !useMultiRecorder {
				continue
			}
			deviceName := exit.Filename
			if device, ok := activeMultiRecordings[exit.Filename]; ok {
				deviceName = device.Name
				delete(activeMultiRecordings, exit.Filename)
			}
			fmt.Printf("Error: %s ffmpeg exited unexpectedly (code %d).\n", deviceName, exit.Code)
			if !multiRecorder.IsRecording() {
				fmt.Println("Waiting for record trigger...")
			}
		case message := <-triggerCh:
			verbosef("OSC %s %v", message.Address, message.Arguments)

			switch message.Address {
			case cfg.OSC.RecordAddress:
				if useMultiRecorder {
					if multiRecorder.IsRecording() {
						fmt.Println("Warning: Record trigger received but already recording. Ignoring.")
						continue
					}

					filenames, err := multiRecorder.Start(
						primary.Mode,
						cfg.Recording.Profile,
						cfg.Recording.Prefix,
						outDir,
						recorder.Slate{
							Show:  cfg.Recording.Show,
							Scene: cfg.Recording.Scene,
							Take:  cfg.Recording.Take,
						},
						verbose,
					)
					if err != nil {
						return err
					}
					activeMultiRecordings = make(map[string]multirecorder.DeviceInfo, len(filenames))
					for i, filename := range filenames {
						activeMultiRecordings[filename] = multiDevices[i]
						fmt.Printf("Recording started: %s\n", filename)
					}
					continue
				}

				if singleRecorder.IsRecording() {
					fmt.Println("Warning: Record trigger received but already recording. Ignoring.")
					continue
				}

				filename, err := singleRecorder.Start(
					primary.Mode,
					cfg.Recording.Profile,
					primary.Selected.VideoConfigValue,
					primary.Selected.AudioConfigValue,
					cfg.Recording.Prefix,
					outDir,
					recorder.Slate{
						Show:  cfg.Recording.Show,
						Scene: cfg.Recording.Scene,
						Take:  cfg.Recording.Take,
					},
					verbose,
				)
				if err != nil {
					return err
				}
				fmt.Printf("Recording started: %s\n", filename)
			case cfg.OSC.StopAddress:
				if useMultiRecorder {
					if !multiRecorder.IsRecording() {
						fmt.Println("Warning: Stop trigger received but not recording. Ignoring.")
						continue
					}

					results := multiRecorder.Stop()
					for _, device := range multiDevices {
						for filename, info := range activeMultiRecordings {
							if info.Name != device.Name {
								continue
							}
							if err := results[filename]; err != nil {
								return err
							}
							fmt.Printf("Recording saved: %s\n", filename)
							delete(activeMultiRecordings, filename)
							break
						}
					}
					fmt.Println("Waiting for record trigger...")
					continue
				}

				if !singleRecorder.IsRecording() {
					fmt.Println("Warning: Stop trigger received but not recording. Ignoring.")
					continue
				}

				exit, err := singleRecorder.StopAndWait(context.Background())
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

func printMultiRunSummary(cfg cfgpkg.Config, devices []resolvedDevice) {
	fmt.Println("osc-record running")
	fmt.Printf("  OSC port:    %d\n", cfg.OSC.Port)
	fmt.Printf("  Record:      %s\n", cfg.OSC.RecordAddress)
	fmt.Printf("  Stop:        %s\n", cfg.OSC.StopAddress)
	fmt.Printf("  Profile:     %s\n", cfg.Recording.Profile)
	fmt.Printf("  Prefix:      %s\n", cfg.Recording.Prefix)
	fmt.Printf("  Output:      %s\n", outputDir(cfg))
	fmt.Println("  Devices:")
	for _, device := range devices {
		fmt.Printf("    - %s (%s)\n", device.Selected.VideoDisplay, device.Mode.Summary())
	}
}

// openPath opens the given file path with the system default application.
func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", path)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}
