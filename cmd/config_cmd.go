package cmd

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Print resolved configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			enc := toml.NewEncoder(os.Stdout)
			return enc.Encode(cfg)
		},
	})
}
