package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	OSC           OSCConfig       `toml:"osc"`
	Device        DeviceConfig    `toml:"device"`
	DevicesConfig []DeviceConfig  `toml:"devices"`
	Recording     RecordingConfig `toml:"recording"`
	FFmpeg        FFmpegConfig    `toml:"ffmpeg"`
	HTTP          HTTPConfig      `toml:"http"`
	TUI           TUIConfig       `toml:"tui"`

	devicesFromArray bool `toml:"-"`
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
	VideoInput  string `toml:"video_input"` // sdi, hdmi, component, composite, s_video, auto (default)
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
	outputDir := "~/Dropbox/osc-record/"
	if runtime.GOOS == "windows" {
		outputDir = "~/Videos/osc-record/"
	}
	return Config{
		OSC: OSCConfig{
			Port:          8000,
			RecordAddress: "/start/record/",
			StopAddress:   "/stop/record/",
		},
		Device: DeviceConfig{
			CaptureMode: "auto",
			Name:        "",
			Audio:       "",
		},
		Recording: RecordingConfig{
			Profile:   "h264",
			Prefix:    "recording",
			OutputDir: outputDir,
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
			cfg.normalizeDevices(false)
			if err := Save(resolvedPath, &cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		return nil, err
	}

	meta, err := toml.DecodeFile(resolvedPath, &cfg)
	if err != nil {
		return nil, err
	}
	cfg.normalizeDevices(meta.IsDefined("devices"))
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	resolvedPath := ExpandPath(path)
	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var buffer bytes.Buffer
	if err := Encode(&buffer, *cfg); err != nil {
		return err
	}

	// Write to a temporary file then rename for crash safety.
	tmpPath := resolvedPath + ".tmp"
	if err := os.WriteFile(tmpPath, buffer.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, resolvedPath)
}

func Encode(w io.Writer, cfg Config) error {
	return toml.NewEncoder(w).Encode(cfg.serializable())
}

func (c Config) ActiveDevices() []DeviceConfig {
	if len(c.DevicesConfig) > 0 {
		return append([]DeviceConfig(nil), c.DevicesConfig...)
	}
	return []DeviceConfig{c.Device}
}

func (c Config) HasMultipleDevices() bool {
	return len(c.ActiveDevices()) > 1
}

func (c Config) UsesDevicesArray() bool {
	return c.devicesFromArray
}

func (c *Config) SetDevices(devices []DeviceConfig, fromArray bool) {
	if len(devices) == 0 {
		devices = []DeviceConfig{Defaults().Device}
	}

	c.DevicesConfig = append([]DeviceConfig(nil), devices...)
	c.Device = c.DevicesConfig[0]
	c.devicesFromArray = fromArray
}

func (c *Config) normalizeDevices(fromArray bool) {
	if len(c.DevicesConfig) > 0 {
		c.SetDevices(c.DevicesConfig, true)
		return
	}
	c.SetDevices([]DeviceConfig{c.Device}, fromArray)
}

func (c Config) serializable() diskConfig {
	out := diskConfig{
		OSC:       c.OSC,
		Recording: c.Recording,
		FFmpeg:    c.FFmpeg,
		HTTP:      c.HTTP,
		TUI:       c.TUI,
	}

	active := c.ActiveDevices()
	if len(active) > 1 || c.devicesFromArray {
		out.DevicesConfig = append([]DeviceConfig(nil), active...)
		return out
	}

	device := active[0]
	out.Device = &device
	return out
}

type diskConfig struct {
	OSC           OSCConfig       `toml:"osc"`
	Device        *DeviceConfig   `toml:"device"`
	DevicesConfig []DeviceConfig  `toml:"devices"`
	Recording     RecordingConfig `toml:"recording"`
	FFmpeg        FFmpegConfig    `toml:"ffmpeg"`
	HTTP          HTTPConfig      `toml:"http"`
	TUI           TUIConfig       `toml:"tui"`
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
