package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	OSC       OSCConfig       `toml:"osc"`
	Device    DeviceConfig    `toml:"device"`
	Recording RecordingConfig `toml:"recording"`
	FFmpeg    FFmpegConfig    `toml:"ffmpeg"`
	HTTP      HTTPConfig      `toml:"http"`
	TUI       TUIConfig       `toml:"tui"`
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
	FormatCode  string `toml:"format_code"`
}

type RecordingConfig struct {
	Profile   string `toml:"profile"`
	Prefix    string `toml:"prefix"`
	OutputDir string `toml:"output_dir"`
	Show      string `toml:"show"`
	Scene     string `toml:"scene"`
	Take      string `toml:"take"`
	PreRoll   int    `toml:"pre_roll"`
}

type FFmpegConfig struct {
	Path string `toml:"path"`
}

type HTTPConfig struct {
	Port int    `toml:"port"`
	Bind string `toml:"bind"`
}

type TUIConfig struct {
	Enabled bool   `toml:"enabled"`
	Theme   string `toml:"theme"`
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
			PreRoll:   0,
		},
		FFmpeg: FFmpegConfig{
			Path: "",
		},
		HTTP: HTTPConfig{
			Port: 0,
			Bind: "0.0.0.0",
		},
		TUI: TUIConfig{
			Enabled: true,
			Theme:   "dark",
		},
	}
}

func ConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "osc-record", "config.toml"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "osc-record", "config.toml"), nil
}

func Load(path string) (*Config, error) {
	resolvedPath := ExpandPath(path)
	cfg := Defaults()

	if _, err := os.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			if err := Save(resolvedPath, &cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		return nil, err
	}

	if _, err := toml.DecodeFile(resolvedPath, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	resolvedPath := ExpandPath(path)
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return err
	}

	var buffer bytes.Buffer
	if err := toml.NewEncoder(&buffer).Encode(cfg); err != nil {
		return err
	}

	return os.WriteFile(resolvedPath, buffer.Bytes(), 0o644)
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
