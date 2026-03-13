package cmd

import (
	"fmt"
	"os"

	"github.com/brodiegraphics/osc-record/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
	verbose bool
	cfg     config.Config
)

var rootCmd = &cobra.Command{
	Use:          "osc-record",
	Short:        "OSC-triggered video capture for live production",
	SilenceUsage: true,
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	defaultCfgPath := config.ConfigPath()
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultCfgPath, "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose logging to stderr")

	cobra.OnInitialize(func() {
		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
	})
}
