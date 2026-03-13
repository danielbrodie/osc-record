package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

type Config struct {
	OSC       OSCConfig       `toml:"osc"`
	Device    DeviceConfig    `toml:"device"`
	Recording RecordingConfig `toml:"recording"`
	FFmpeg    FFmpegConfig    `toml:"ffmpeg"`
}

type OSCConfig struct {
	Port          int    `toml:"port"`
	RecordAddress string `toml:"record_address"`
	StopAddress   string `toml:"stop_address"`
}

type DeviceConfig struct {
	CaptureMode string `toml:"capture_mode"`
	Name        string `toml:"name"`
	Audio       string `toml:"audio"`
}

type RecordingConfig struct {
	Profile   string `toml:"profile"`
	Prefix    string `toml:"prefix"`
	OutputDir string `toml:"output_dir"`
}

type FFmpegConfig struct {
	Path string `toml:"path"`
}

func Defaults() Config {
	return Config{
		OSC: OSCConfig{
			Port:          8000,
			RecordAddress: "",
			StopAddress:   "",
		},
		Device: DeviceConfig{
			CaptureMode: "auto",
			Name:        "",
			Audio:       "",
		},
		Recording: RecordingConfig{
			Profile:   "h264",
			Prefix:    "recording",
			OutputDir: "~/Dropbox/osc-record/",
		},
		FFmpeg: FFmpegConfig{
			Path: "",
		},
	}
}

func ConfigPath() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		return filepath.Join(appData, "osc-record", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "osc-record", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err2 := Save(path, cfg); err2 != nil {
			return cfg, err2
		}
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}

func ExpandTilde(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
