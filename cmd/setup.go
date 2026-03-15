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
		fmt.Print("Send your RECORD cue now (or Enter to keep current)... ")
		addr, err := listenForOSC(cfg.OSC.Port, 15*time.Second)
		if err != nil {
			if isPortInUse(err) {
				fmt.Printf("\nError: %s\n", err)
				return err
			} else if cfg.OSC.RecordAddress == "" {
				fmt.Println("\nNo OSC received. Configure manually in config.toml.")
			} else {
				fmt.Printf("\nTimeout — keeping %q\n", cfg.OSC.RecordAddress)
			}
		} else {
			fmt.Printf("\nReceived: %s\n", addr)
			fmt.Printf("Use %q as record trigger? [Y/n]: ", addr)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(strings.ToLower(line))
			if line == "" || line == "y" {
				cfg.OSC.RecordAddress = addr
				fmt.Printf("✓ Record address: %s\n", addr)
			}
		}
		fmt.Println()

		// Step 2: OSC stop address
		fmt.Printf("Current stop address: %q\n", cfg.OSC.StopAddress)
		fmt.Print("Send your STOP cue now (or Enter to keep current)... ")
		addr, err = listenForOSC(cfg.OSC.Port, 15*time.Second)
		if err != nil {
			if isPortInUse(err) {
				fmt.Printf("\nError: %s\n", err)
				return err
			} else if cfg.OSC.StopAddress == "" {
				fmt.Println("\nNo OSC received. Configure manually in config.toml.")
			} else {
				fmt.Printf("\nTimeout — keeping %q\n", cfg.OSC.StopAddress)
			}
		} else {
			fmt.Printf("\nReceived: %s\n", addr)
			fmt.Printf("Use %q as stop trigger? [Y/n]: ", addr)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(strings.ToLower(line))
			if line == "" || line == "y" {
				cfg.OSC.StopAddress = addr
				fmt.Printf("✓ Stop address: %s\n", addr)
			}
		}
		fmt.Println()

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
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return "", err
		}
		// Minimal OSC address parse: OSC packets start with '/'
		pkt, err := osc.ParsePacket(string(buf[:n]))
		if err != nil {
			continue
		}
		if msg, ok := pkt.(*osc.Message); ok && strings.HasPrefix(msg.Address, "/") {
			return msg.Address, nil
		}
	}
}

// cfgPath returns the resolved config path.
func cfgPath() string {
	p, _ := cfgpkg.ConfigPath()
	return p
}
