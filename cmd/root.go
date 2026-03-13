package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	cfgpkg "github.com/danielbrodie/osc-record/internal/config"
)

var (
	rootCmd = &cobra.Command{
		Use:           "osc-record",
		Short:         "OSC-triggered video capture for live production",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cfgpkg.Load(configFile)
			if err != nil {
				return err
			}
			loadedConfig = cfg
			return nil
		},
	}

	loadedConfig *cfgpkg.Config
	configFile   string
	verbose      bool
	appVersion   = "0.1.0"
)

func init() {
	defaultConfigPath, err := cfgpkg.ConfigPath()
	if err != nil {
		defaultConfigPath = filepath.Join(".", "config.toml")
	}

	rootCmd.PersistentFlags().StringVar(&configFile, "config", defaultConfigPath, "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose logging to stderr")
}

func Execute() error {
	return rootCmd.Execute()
}

func SetVersion(version string) {
	appVersion = version
}

func mustConfig() cfgpkg.Config {
	if loadedConfig == nil {
		return cfgpkg.Defaults()
	}
	return *loadedConfig
}

func saveConfig(cfg cfgpkg.Config) error {
	if err := cfgpkg.Save(configFile, &cfg); err != nil {
		return err
	}
	loadedConfig = &cfg
	return nil
}

func resolveFFmpegPath(cfg cfgpkg.Config) (string, error) {
	if cfg.FFmpeg.Path != "" {
		path := cfgpkg.ExpandPath(cfg.FFmpeg.Path)
		if resolved, err := exec.LookPath(path); err == nil {
			return resolved, nil
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("Error: ffmpeg not found at %s.", path)
	}

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", errors.New("Error: ffmpeg not found on PATH. Install with 'brew install ffmpeg' or set ffmpeg.path in config.")
	}
	return path, nil
}

func outputDir(cfg cfgpkg.Config) string {
	return cfgpkg.ExpandPath(cfg.Recording.OutputDir)
}

func verbosef(format string, args ...any) {
	if !verbose {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
