package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/brodiegraphics/osc-record/internal/capture"
	"github.com/brodiegraphics/osc-record/internal/config"
	"github.com/brodiegraphics/osc-record/internal/devices"
	osclib "github.com/brodiegraphics/osc-record/internal/osc"
	"github.com/brodiegraphics/osc-record/internal/platform"
	"github.com/brodiegraphics/osc-record/internal/recorder"
	"github.com/spf13/cobra"
)

func init() {
	var (
		prefix      string
		profile     string
		output      string
		port        int
		captureMode string
		videoDevice string
		audioDevice string
	)

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start the OSC recording daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ffmpeg := devices.FFmpegPath(cfg.FFmpeg.Path)

			// 1. Validate ffmpeg
			if _, err := exec.LookPath(ffmpeg); err != nil {
				if _, err2 := os.Stat(ffmpeg); err2 != nil {
					return fmt.Errorf("Error: ffmpeg not found on PATH. Install with 'brew install ffmpeg' or set ffmpeg.path in config.")
				}
			}

			// 2. Validate OSC addresses
			if cfg.OSC.RecordAddress == "" {
				return fmt.Errorf("Error: No record trigger configured. Run 'osc-record capture record' first.")
			}
			if cfg.OSC.StopAddress == "" {
				return fmt.Errorf("Error: No stop trigger configured. Run 'osc-record capture stop' first.")
			}

			// Apply flag overrides
			if cmd.Flags().Changed("port") {
				cfg.OSC.Port = port
			}
			if cmd.Flags().Changed("profile") {
				cfg.Recording.Profile = profile
			}
			if cmd.Flags().Changed("prefix") {
				cfg.Recording.Prefix = prefix
			}
			if cmd.Flags().Changed("output") {
				cfg.Recording.OutputDir = output
			}
			if cmd.Flags().Changed("capture-mode") {
				cfg.Device.CaptureMode = captureMode
			}
			if cmd.Flags().Changed("video-device") {
				cfg.Device.Name = videoDevice
			}
			if cmd.Flags().Changed("audio-device") {
				cfg.Device.Audio = audioDevice
			}

			// 3. Resolve capture mode (automatic, decklink wins)
			resolvedMode := capture.ResolveMode(ffmpeg, cfg.Device.CaptureMode)

			// 4. Interactive device picker if device.name unset
			if cfg.Device.Name == "" && !cmd.Flags().Changed("video-device") {
				if err := pickDevice(ffmpeg, resolvedMode); err != nil {
					return err
				}
			}

			// Windows ProRes warning
			if runtime.GOOS == "windows" && strings.ToLower(cfg.Recording.Profile) == "prores" {
				fmt.Fprintln(os.Stderr, "Warning: ProRes playback on Windows requires QuickTime or VLC.")
			}

			// Build capture mode
			var capMode capture.Mode
			switch resolvedMode {
			case "decklink":
				capMode = &capture.DecklinkMode{DeviceName: cfg.Device.Name}
			default:
				capMode = buildFallbackMode(cfg.Device.Name, cfg.Device.Audio)
			}

			// 5. Decklink probe (before binding OSC port)
			if resolvedMode == "decklink" {
				probe := exec.Command(ffmpeg, "-f", "decklink", "-i", cfg.Device.Name, "-t", "2", "-f", "null", "-")
				if err := probe.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: No valid signal detected on %q. Recording will fail until a signal is present.\n", cfg.Device.Name)
				}
			}

			// 6. Create output dir
			outDir := config.ExpandTilde(cfg.Recording.OutputDir)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("Error: Output directory %s does not exist and could not be created: %v", outDir, err)
			}

			// 7. Print startup summary
			recordAddr := cfg.OSC.RecordAddress
			stopAddr := cfg.OSC.StopAddress
			oscPort := cfg.OSC.Port

			fmt.Println("osc-record running")
			fmt.Printf("  OSC port:    %d\n", oscPort)
			fmt.Printf("  Record:      %s\n", recordAddr)
			fmt.Printf("  Stop:        %s\n", stopAddr)
			fmt.Printf("  Capture:     %s\n", capMode.Name())
			fmt.Printf("  Profile:     %s\n", cfg.Recording.Profile)
			fmt.Printf("  Prefix:      %s\n", cfg.Recording.Prefix)
			fmt.Printf("  Output:      %s\n", outDir)
			fmt.Printf("  Device:      %s\n", cfg.Device.Name)
			fmt.Println()
			fmt.Println("Waiting for record trigger...")

			rec := &recorder.Recorder{
				FFmpegPath: ffmpeg,
				OutputDir:  outDir,
				Prefix:     cfg.Recording.Prefix,
				Profile:    cfg.Recording.Profile,
				Mode:       capMode,
				Stopper:    platform.New(),
			}

			var mu sync.Mutex

			srv := osclib.NewServer(oscPort, func(addr string, _ []interface{}) {
				mu.Lock()
				defer mu.Unlock()

				switch addr {
				case recordAddr:
					if rec.IsRecording() {
						fmt.Fprintln(os.Stderr, "Warning: Record trigger received but already recording. Ignoring.")
						return
					}
					f, startErr := rec.Start()
					if startErr != nil {
						fmt.Fprintf(os.Stderr, "Error starting recording: %v\n", startErr)
						return
					}
					fmt.Printf("Recording started: %s\n", f)
					rec.WatchExit(func(code int) {
						fmt.Fprintf(os.Stderr, "Error: ffmpeg exited unexpectedly (code %d). Waiting for record trigger...\n", code)
					})

				case stopAddr:
					if !rec.IsRecording() {
						fmt.Fprintln(os.Stderr, "Warning: Stop trigger received but not recording. Ignoring.")
						return
					}
					f, stopErr := rec.Stop()
					if stopErr != nil {
						fmt.Fprintf(os.Stderr, "Error stopping recording: %v\n", stopErr)
						return
					}
					fmt.Printf("Recording saved: %s\n", f)
					fmt.Println("Waiting for record trigger...")
				}
			})

			// Handle SIGTERM/SIGINT gracefully
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				mu.Lock()
				if rec.IsRecording() {
					f, _ := rec.Stop()
					if f != "" {
						fmt.Printf("\nRecording saved: %s\n", f)
					}
				}
				mu.Unlock()
				os.Exit(0)
			}()

			return srv.ListenAndServe()
		},
	}

	runCmd.Flags().StringVar(&prefix, "prefix", "recording", "Filename prefix")
	runCmd.Flags().StringVar(&profile, "profile", "h264", "Recording profile: prores, hevc, h264")
	runCmd.Flags().StringVar(&output, "output", "~/Dropbox/osc-record/", "Output directory")
	runCmd.Flags().IntVar(&port, "port", 8000, "OSC listen port")
	runCmd.Flags().StringVar(&captureMode, "capture-mode", "auto", "Capture mode: auto, decklink, avfoundation, dshow")
	runCmd.Flags().StringVar(&videoDevice, "video-device", "", "Override video device")
	runCmd.Flags().StringVar(&audioDevice, "audio-device", "", "Override audio device")

	rootCmd.AddCommand(runCmd)
}

func pickDevice(ffmpeg, resolvedMode string) error {
	scanner := bufio.NewScanner(os.Stdin)

	if resolvedMode == "decklink" {
		dl := devices.ListDecklink(ffmpeg)
		if len(dl) == 0 {
			return fmt.Errorf("Error: No capture devices found. Run 'osc-record devices' for details.")
		}
		if len(dl) == 1 {
			cfg.Device.Name = dl[0]
			fmt.Printf("Auto-selected device: %s\n", cfg.Device.Name)
		} else {
			fmt.Println("No capture device configured. Available devices:")
			fmt.Println()
			for i, d := range dl {
				fmt.Printf("  [%d] %s\n", i+1, d)
			}
			fmt.Println()
			fmt.Printf("Select device [1-%d]: ", len(dl))
			scanner.Scan()
			n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
			if err != nil || n < 1 || n > len(dl) {
				return fmt.Errorf("invalid selection")
			}
			cfg.Device.Name = dl[n-1]
		}
	} else {
		var video, audio []string
		if runtime.GOOS == "windows" {
			video, audio = devices.ListDShow(ffmpeg)
		} else {
			video, audio = devices.ListAVFoundation(ffmpeg)
		}
		if len(video) == 0 {
			return fmt.Errorf("Error: No capture devices found. Run 'osc-record devices' for details.")
		}
		if len(video) == 1 {
			cfg.Device.Name = video[0]
			fmt.Printf("Auto-selected video device: %s\n", cfg.Device.Name)
		} else {
			fmt.Println("No capture device configured. Available video devices:")
			fmt.Println()
			for i, d := range video {
				fmt.Printf("  [%d] %s\n", i+1, d)
			}
			fmt.Println()
			fmt.Printf("Select video device [1-%d]: ", len(video))
			scanner.Scan()
			n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
			if err != nil || n < 1 || n > len(video) {
				return fmt.Errorf("invalid selection")
			}
			cfg.Device.Name = video[n-1]
		}
		if len(audio) == 1 {
			cfg.Device.Audio = audio[0]
			fmt.Printf("Auto-selected audio device: %s\n", cfg.Device.Audio)
		} else if len(audio) > 1 {
			fmt.Println()
			fmt.Println("Available audio devices:")
			fmt.Println()
			for i, d := range audio {
				fmt.Printf("  [%d] %s\n", i+1, d)
			}
			fmt.Println()
			fmt.Printf("Select audio device [1-%d]: ", len(audio))
			scanner.Scan()
			n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
			if err != nil || n < 1 || n > len(audio) {
				return fmt.Errorf("invalid selection")
			}
			cfg.Device.Audio = audio[n-1]
		}
	}

	return config.Save(cfgPath, cfg)
}

func buildFallbackMode(videoDevice, audioDevice string) capture.Mode {
	if runtime.GOOS == "windows" {
		return &capture.DShowMode{VideoDevice: videoDevice, AudioDevice: audioDevice}
	}
	return &capture.AVFoundationMode{VideoDevice: videoDevice, AudioDevice: audioDevice}
}
