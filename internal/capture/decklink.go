package capture

import (
	"context"
	"os/exec"
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
