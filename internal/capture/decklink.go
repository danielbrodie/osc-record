package capture

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

type DecklinkMode struct {
	FormatCode string
	VideoInput string // sdi, hdmi, component, composite, s_video — empty means auto
}

func (DecklinkMode) Name() string {
	return ModeDecklink
}

func (DecklinkMode) Summary() string {
	return "decklink (auto-detect)"
}

func (m DecklinkMode) buildInputModifiers() []string {
	var args []string
	if m.VideoInput != "" && m.VideoInput != "auto" {
		args = append(args, "-video_input", m.VideoInput)
	}
	if m.FormatCode != "" {
		args = append(args, "-format_code", m.FormatCode)
	}
	return args
}

func (m DecklinkMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	args := []string{"-f", "decklink"}
	args = append(args, m.buildInputModifiers()...)
	args = append(args, "-i", videoDevice)
	return args
}

// BuildExternalAudioArgs returns ffmpeg input args for a secondary audio source alongside
// DeckLink video. On macOS, uses avfoundation (e.g. ":Dante Virtual Soundcard" or ":1").
// On Windows, uses dshow (e.g. "Dante Virtual Soundcard"). On Linux, uses pulse.
// Returns nil when audioDevice is empty (use DeckLink embedded audio).
func (DecklinkMode) BuildExternalAudioArgs(audioDevice string) []string {
	if audioDevice == "" {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		// avfoundation audio-only input: ":deviceName" or ":index"
		input := audioDevice
		if len(audioDevice) > 0 && audioDevice[0] != ':' {
			input = ":" + audioDevice
		}
		return []string{"-f", "avfoundation", "-i", input}
	case "windows":
		return []string{"-f", "dshow", "-i", "audio=" + audioDevice}
	default: // linux
		return []string{"-f", "pulse", "-i", audioDevice}
	}
}

func (DecklinkMode) NeedsAudio() bool {
	return false
}

func (m DecklinkMode) SignalProbe(ffmpegPath, device string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	args := []string{"-f", "decklink"}
	args = append(args, m.buildInputModifiers()...)
	args = append(args, "-i", device, "-t", "2", "-f", "null", "-")
	return exec.CommandContext(ctx, ffmpegPath, args...).Run()
}
