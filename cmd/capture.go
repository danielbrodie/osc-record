package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"github.com/brodiegraphics/osc-record/internal/config"
	osclib "github.com/brodiegraphics/osc-record/internal/osc"
	"github.com/spf13/cobra"
)

func init() {
	captureCmd := &cobra.Command{
		Use:   "capture",
		Short: "Configure OSC trigger addresses",
	}
	captureCmd.AddCommand(makeCaptureLearnCmd("record"))
	captureCmd.AddCommand(makeCaptureLearnCmd("stop"))
	rootCmd.AddCommand(captureCmd)
}

func makeCaptureLearnCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Learn the OSC address for the %s trigger", name),
		RunE: func(cmd *cobra.Command, args []string) error {
			port := cfg.OSC.Port
			fmt.Printf("Listening for OSC on port %d... Press Enter to select, Ctrl+C to cancel.\n\n", port)

			var mu sync.Mutex
			var lastAddr string

			srv := osclib.NewServer(port, func(addr string, _ []interface{}) {
				mu.Lock()
				lastAddr = addr
				mu.Unlock()
				fmt.Printf("  %s []        <-- most recent\n", addr)
			})
			go srv.ListenAndServe() //nolint:errcheck

			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()

			mu.Lock()
			selected := lastAddr
			mu.Unlock()

			if selected == "" {
				return fmt.Errorf("no OSC message received")
			}

			switch name {
			case "record":
				cfg.OSC.RecordAddress = selected
			case "stop":
				cfg.OSC.StopAddress = selected
			}

			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("could not save config: %w", err)
			}

			switch name {
			case "record":
				fmt.Printf("Saved record trigger: %s\n", selected)
			case "stop":
				fmt.Printf("Saved stop trigger: %s\n", selected)
			}
			return nil
		},
	}
}
