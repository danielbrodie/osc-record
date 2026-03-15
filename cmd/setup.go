package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/hypebeast/go-osc/osc"
	"github.com/spf13/cobra"
	"github.com/danielbrodie/osc-record/internal/capture"
	cfgpkg "github.com/danielbrodie/osc-record/internal/config"
	"github.com/danielbrodie/osc-record/internal/devices"
)

func init() {
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard (plaintext, no TUI required)",
	Long: `Walks through device, OSC, and output configuration.
Saves results to the config file. Use this for scripted or headless environments.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := mustConfig()
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("osc-record setup")
		fmt.Println(strings.Repeat("─", 40))
		fmt.Println()

		ffmpegPath, err := resolveFFmpegPath(cfg)
		if err != nil {
			return err
		}

		// Step 1: Detect capture mode and probe devices.
		devs := cfg.ActiveDevices()
		deviceCfg := devs[0]
		mode, _, err := capture.ResolveMode(deviceCfg.CaptureMode, ffmpegPath, runtime.GOOS, deviceCfg.FormatCode, deviceCfg.VideoInput, deviceCfg.DShowVideoSize, deviceCfg.DShowFramerate)
		if err != nil {
			return err
		}

		group, err := devices.ProbeMode(ffmpegPath, mode.Name())
		if err != nil {
			return fmt.Errorf("failed to probe devices: %w", err)
		}

		// Step 2: Video device selection.
		fmt.Printf("Capture mode: %s\n\n", mode.Summary())
		video, err := promptForDevice(group.Video, "capture device", mode.Name() == capture.ModeDecklink)
		if err != nil {
			return err
		}
		deviceCfg.Name = video.ConfigValue()
		fmt.Printf("✓ Video device: %s\n\n", video.Name)

		// Step 3: Audio device (dshow and avfoundation require explicit audio).
		if mode.NeedsAudio() {
			if matched, matchErr := devices.BestAudioMatch(group.Audio, video.Name); matchErr == nil {
				deviceCfg.Audio = matched.ConfigValue()
				fmt.Printf("✓ Audio device: %s (auto-matched)\n\n", matched.Name)
			} else {
				audio, promptErr := promptForDevice(group.Audio, "audio device", false)
				if promptErr != nil {
					return promptErr
				}
				deviceCfg.Audio = audio.ConfigValue()
				fmt.Printf("✓ Audio device: %s\n\n", audio.Name)
			}
		}

		// Step 4: Video input — only relevant for DeckLink (HDMI vs SDI).
		var videoInput string
		if mode.Name() == capture.ModeDecklink {
			fmt.Printf("Video input [1=HDMI / 2=SDI / 3=Auto-detect (default)]: ")
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			switch line {
			case "1", "hdmi", "HDMI":
				videoInput = "hdmi"
				fmt.Printf("✓ Video input: HDMI\n\n")
			case "2", "sdi", "SDI":
				videoInput = "sdi"
				fmt.Printf("✓ Video input: SDI\n\n")
			default:
				videoInput = ""
				fmt.Printf("✓ Video input: Auto-detect (on next run)\n\n")
			}
		}
		deviceCfg.VideoInput = videoInput
		devs[0] = deviceCfg
		cfg.SetDevices(devs, cfg.UsesDevicesArray())

		// Step 5: OSC record address.
		fmt.Printf("Current record address: %q\n", cfg.OSC.RecordAddress)
		if addr := captureOSCAddress(reader, cfg.OSC.Port, "RECORD", 60*time.Second); addr != "" {
			cfg.OSC.RecordAddress = addr
			fmt.Printf("✓ Record address: %s\n\n", addr)
		} else if cfg.OSC.RecordAddress != "" {
			fmt.Printf("Keeping %q\n\n", cfg.OSC.RecordAddress)
		} else {
			fmt.Println("Skipped — no record address set.\n")
		}

		// Step 6: OSC stop address.
		fmt.Printf("Current stop address: %q\n", cfg.OSC.StopAddress)
		if addr := captureOSCAddress(reader, cfg.OSC.Port, "STOP", 60*time.Second); addr != "" {
			cfg.OSC.StopAddress = addr
			fmt.Printf("✓ Stop address: %s\n\n", addr)
		} else if cfg.OSC.StopAddress != "" {
			fmt.Printf("Keeping %q\n\n", cfg.OSC.StopAddress)
		} else {
			fmt.Println("Skipped — no stop address set.\n")
		}

		// Step 7: Output directory.
		fmt.Printf("Output directory [%s]: ", cfg.Recording.OutputDir)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.Recording.OutputDir = line
		}
		fmt.Printf("✓ Output: %s\n\n", cfg.Recording.OutputDir)

		// Step 8: Filename prefix.
		fmt.Printf("Filename prefix [%s]: ", cfg.Recording.Prefix)
		line, _ = reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.Recording.Prefix = line
		}
		fmt.Printf("✓ Prefix: %s\n\n", cfg.Recording.Prefix)

		// Save.
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println(strings.Repeat("─", 40))
		fmt.Println("Config saved. Run 'osc-record run' to start.")
		return nil
	},
}

// listenForOSC opens a UDP socket and returns the first OSC address received within timeout.
// captureOSCAddress tries to receive an OSC packet for timeout, then falls back
// to prompting the user to type the address manually. Returns "" if skipped.
func captureOSCAddress(reader *bufio.Reader, port int, label string, timeout time.Duration) string {
	fmt.Printf("Listening on port %d for %s — send your %s cue now, or type it manually and press Enter: ", port, timeout.Round(time.Second), label)

	// Channel for OSC packet received from network
	oscCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		addr, err := listenForOSC(port, timeout)
		if err != nil {
			errCh <- err
		} else {
			oscCh <- addr
		}
	}()

	// Channel for manual keyboard input
	inputCh := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		inputCh <- line
	}()

	select {
	case addr := <-oscCh:
		fmt.Printf("\n  → received: %s\n", addr)
		return addr
	case line := <-inputCh:
		if line == "" {
			return ""
		}
		if !strings.HasPrefix(line, "/") {
			line = "/" + line
		}
		return line
	case err := <-errCh:
		if isPortInUse(err) {
			fmt.Printf("\nWarning: %s\n", err)
		} else {
			fmt.Printf("\nNo OSC received (timeout).\n")
		}
		fmt.Printf("Type the %s address manually (or press Enter to skip): ", label)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return ""
		}
		if !strings.HasPrefix(line, "/") {
			line = "/" + line
		}
		return line
	}
}

func isPortInUse(err error) bool {
	return err != nil && strings.Contains(err.Error(), "address already in use")
}

func listenForOSC(port int, timeout time.Duration) (string, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)

	// Use SO_REUSEPORT so the wizard can co-exist with Protokol or other OSC monitors.
	lc := reusePortListenConfig()
	pc, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		pc, err = net.ListenPacket("udp", addr)
	}
	if err != nil {
		if isPortInUse(err) {
			return "", fmt.Errorf("port %d is already in use — stop osc-record run before running setup", port)
		}
		return "", err
	}
	conn := pc
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", err
	}

	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFrom(buf)
		if err != nil {
			return "", err
		}
		// Minimal OSC address parse: OSC packets start with '/'
		pkt, err := osc.ParsePacket(string(buf[:n]))
		if err != nil {
			fmt.Printf("  (received non-OSC packet from %s, ignoring)\n", src)
			continue
		}
		switch p := pkt.(type) {
		case *osc.Message:
			if strings.HasPrefix(p.Address, "/") {
				fmt.Printf("  → received: %s (from %s)\n", p.Address, src)
				return p.Address, nil
			}
		case *osc.Bundle:
			// Disguise (and other systems) wrap messages in OSC bundles.
			for _, msg := range p.Messages {
				if strings.HasPrefix(msg.Address, "/") {
					fmt.Printf("  → received: %s (from %s, in bundle)\n", msg.Address, src)
					return msg.Address, nil
				}
			}
		}
	}
}

// cfgPath returns the resolved config path.
func cfgPath() string {
	p, _ := cfgpkg.ConfigPath()
	return p
}
