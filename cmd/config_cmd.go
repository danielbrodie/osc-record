package cmd

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the resolved configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := mustConfig()
		return toml.NewEncoder(os.Stdout).Encode(cfg)
	},
}
