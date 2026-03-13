package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/danielbrodie/osc-record/internal/devices"
)

func init() {
	rootCmd.AddCommand(devicesCmd)
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List available capture devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := mustConfig()
		ffmpegPath, err := resolveFFmpegPath(cfg)
		if err != nil {
			return err
		}

		groups, err := devices.ProbeForPlatform(ffmpegPath, runtime.GOOS)
		if err != nil {
			return err
		}

		for i, group := range groups {
			if i > 0 {
				fmt.Println()
			}

			fmt.Printf("Capture mode: %s\n\n", group.ModeDescription())
			switch group.Mode {
			case devices.ModeDecklink:
				fmt.Println("DeckLink devices:")
				if len(group.Video) == 0 {
					fmt.Println("  (none found)")
					continue
				}
				for _, item := range group.Video {
					fmt.Printf("  %s\n", item.Name)
				}
			default:
				fmt.Println("Video devices:")
				if len(group.Video) == 0 {
					fmt.Println("  (none found)")
				} else {
					for _, item := range group.Video {
						fmt.Printf("  [%s] %s\n", item.ID, item.Name)
					}
				}
				fmt.Println()
				fmt.Println("Audio devices:")
				if len(group.Audio) == 0 {
					fmt.Println("  (none found)")
					continue
				}
				for _, item := range group.Audio {
					if item.ID == "" {
						fmt.Printf("  %s\n", item.Name)
						continue
					}
					fmt.Printf("  [%s] %s\n", item.ID, item.Name)
				}
			}
		}

		return nil
	},
}
