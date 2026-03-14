package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/danielbrodie/osc-record/internal/config"
	"github.com/danielbrodie/osc-record/internal/manifest"
)

func init() {
	rootCmd.AddCommand(manifestCmd)
}

var manifestCmd = &cobra.Command{
	Use:   "manifest [dir]",
	Short: "Generate a session manifest for a directory of clips",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = config.ExpandPath(args[0])
		}

		cfg := mustConfig()
		entries, err := manifest.BuildEntriesFromDir(dir)
		if err != nil {
			return err
		}

		content := manifest.RenderForDirectory(entries, cfg, dir)
		filename := manifest.ManifestFilename(cfg.Recording.Show, mustNow())
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			return err
		}

		fmt.Print(content)
		return nil
	},
}

func mustNow() time.Time {
	return time.Now()
}
