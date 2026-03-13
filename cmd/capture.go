package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	oscpkg "github.com/danielbrodie/osc-record/internal/osc"
)

func init() {
	captureCmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture OSC trigger addresses",
	}

	captureCmd.AddCommand(
		newCaptureAddressCmd("record", "record"),
		newCaptureAddressCmd("stop", "stop"),
	)

	rootCmd.AddCommand(captureCmd)
}

func newCaptureAddressCmd(name, field string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Capture the %s OSC address", name),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := mustConfig()

			fmt.Printf("Listening for OSC on port %d... Press Enter to select, Ctrl+C to cancel.\n\n", cfg.OSC.Port)

			var (
				mu          sync.Mutex
				lastAddress string
			)

			listener, err := oscpkg.Listen(cfg.OSC.Port, func(message oscpkg.Message) {
				mu.Lock()
				lastAddress = message.Address
				mu.Unlock()

				fmt.Printf("  %s %v        <-- most recent\n", message.Address, message.Arguments)
			})
			if err != nil {
				return err
			}
			defer listener.Close()

			reader := bufio.NewReader(os.Stdin)
			if _, err := reader.ReadString('\n'); err != nil {
				return err
			}

			mu.Lock()
			address := lastAddress
			mu.Unlock()
			if address == "" {
				return errors.New("Error: No OSC messages received. Nothing saved.")
			}

			switch field {
			case "record":
				cfg.OSC.RecordAddress = address
			case "stop":
				cfg.OSC.StopAddress = address
			default:
				return errors.New("unsupported capture field")
			}

			if err := saveConfig(cfg); err != nil {
				return err
			}

			switch field {
			case "record":
				fmt.Printf("Saved record trigger: %s\n", address)
			case "stop":
				fmt.Printf("Saved stop trigger: %s\n", address)
			}
			return nil
		},
	}
}
