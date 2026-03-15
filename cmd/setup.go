package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hypebeast/go-osc/osc"
	"github.com/spf13/cobra"
	cfgpkg "github.com/danielbrodie/osc-record/internal/config"
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

		// Step 1: OSC record address
		fmt.Printf("Current record address: %q\n", cfg.OSC.RecordAddress)
		if addr := captureOSCAddress(reader, cfg.OSC.Port, "RECORD", 60*time.Second); addr != "" {
			cfg.OSC.RecordAddress = addr
			fmt.Printf("✓ Record address: %s\n\n", addr)
		} else if cfg.OSC.RecordAddress != "" {
			fmt.Printf("Keeping %q\n\n", cfg.OSC.RecordAddress)
		} else {
			fmt.Println("Skipped — no record address set.\n")
		}

		// Step 2: OSC stop address
		fmt.Printf("Current stop address: %q\n", cfg.OSC.StopAddress)
		if addr := captureOSCAddress(reader, cfg.OSC.Port, "STOP", 60*time.Second); addr != "" {
			cfg.OSC.StopAddress = addr
			fmt.Printf("✓ Stop address: %s\n\n", addr)
		} else if cfg.OSC.StopAddress != "" {
			fmt.Printf("Keeping %q\n\n", cfg.OSC.StopAddress)
		} else {
			fmt.Println("Skipped — no stop address set.\n")
		}

		// Step 3: Output directory
		fmt.Printf("Output directory [%s]: ", cfg.Recording.OutputDir)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.Recording.OutputDir = line
		}
		fmt.Printf("✓ Output: %s\n\n", cfg.Recording.OutputDir)

		// Step 4: Prefix
		fmt.Printf("Filename prefix [%s]: ", cfg.Recording.Prefix)
		line, _ = reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.Recording.Prefix = line
		}
		fmt.Printf("✓ Prefix: %s\n\n", cfg.Recording.Prefix)

		// Save
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
		if msg, ok := pkt.(*osc.Message); ok && strings.HasPrefix(msg.Address, "/") {
			fmt.Printf("  → received: %s (from %s)\n", msg.Address, src)
			return msg.Address, nil
		}
	}
}

// cfgPath returns the resolved config path.
func cfgPath() string {
	p, _ := cfgpkg.ConfigPath()
	return p
}
