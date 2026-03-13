package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/brodiegraphics/osc-record/internal/capture"
	cfgpkg "github.com/brodiegraphics/osc-record/internal/config"
	"github.com/brodiegraphics/osc-record/internal/devices"
	oscpkg "github.com/brodiegraphics/osc-record/internal/osc"
	"github.com/brodiegraphics/osc-record/internal/platform"
	"github.com/brodiegraphics/osc-record/internal/recorder"
)

func init() {
	defaults := cfgpkg.Defaults()

	runCmd.Flags().String("prefix", defaults.Recording.Prefix, "Filename prefix prepended to date")
	runCmd.Flags().String("profile", defaults.Recording.Profile, "Recording profile: prores, hevc, or h264")
	runCmd.Flags().String("output", defaults.Recording.OutputDir, "Output directory for recordings")
	runCmd.Flags().Int("port", defaults.OSC.Port, "Override OSC listen port")
	runCmd.Flags().String("capture-mode", defaults.Device.CaptureMode, "Capture mode: auto, decklink, avfoundation, or dshow")
	runCmd.Flags().String("video-device", "", "Override video device (index or name)")
	runCmd.Flags().String("audio-device", "", "Override audio device (index or name)")

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
