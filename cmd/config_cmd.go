package cmd

import (
	"os"

	"github.com/spf13/cobra"

	cfgpkg "github.com/danielbrodie/osc-record/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the resolved configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := mustConfig()
		return cfgpkg.Encode(os.Stdout, cfg)
	},
}
