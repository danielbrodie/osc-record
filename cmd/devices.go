package cmd

import (
	"fmt"
	"runtime"

	"github.com/brodiegraphics/osc-record/internal/capture"
	"github.com/brodiegraphics/osc-record/internal/devices"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "devices",
		Short: "List available capture devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			ffmpeg := devices.FFmpegPath(cfg.FFmpeg.Path)
			decklinked := false

			if capture.SupportsDecklink(ffmpeg) {
				dl := devices.ListDecklink(ffmpeg)
				fmt.Println("Capture mode: decklink (auto-detect signal format)")
				fmt.Println()
				fmt.Println("DeckLink devices:")
				for _, d := range dl {
					fmt.Printf("  %s\n", d)
				}
				fmt.Println()
				decklinked = true
			}

			if runtime.GOOS == "windows" {
				video, audio := devices.ListDShow(ffmpeg)
				if !decklinked || len(video) > 0 {
					fmt.Println("Capture mode: dshow (manual format required)")
					fmt.Println()
					fmt.Println("Video devices:")
					for i, d := range video {
						fmt.Printf("  [%d] %s\n", i, d)
					}
					fmt.Println()
					fmt.Println("Audio devices:")
					for i, d := range audio {
						fmt.Printf("  [%d] %s\n", i, d)
					}
					fmt.Println()
				}
			} else {
				video, audio := devices.ListAVFoundation(ffmpeg)
				if !decklinked || len(video) > 0 {
					fmt.Println("Capture mode: avfoundation (manual format required)")
					fmt.Println()
					fmt.Println("Video devices:")
					for i, d := range video {
						fmt.Printf("  [%d] %s\n", i, d)
					}
					fmt.Println()
					fmt.Println("Audio devices:")
					for i, d := range audio {
						fmt.Printf("  [%d] %s\n", i, d)
					}
					fmt.Println()
				}
			}
			return nil
		},
	})
}
