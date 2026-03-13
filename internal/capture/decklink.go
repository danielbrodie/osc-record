package capture

import "os/exec"

type DecklinkMode struct {
	FormatCode string
}

func (DecklinkMode) Name() string {
	return ModeDecklink
}

func (DecklinkMode) Summary() string {
	return "decklink (auto-detect)"
}

func (m DecklinkMode) buildFormatArgs() []string {
	if m.FormatCode != "" {
		return []string{"-format_code", m.FormatCode}
	}
	return nil
}

func (m DecklinkMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	args := []string{"-f", "decklink"}
	args = append(args, m.buildFormatArgs()...)
	args = append(args, "-i", videoDevice)
	return args
}

func (DecklinkMode) NeedsAudio() bool {
	return false
}

func (m DecklinkMode) SignalProbe(ffmpegPath, device string) error {
	args := []string{"-f", "decklink"}
	args = append(args, m.buildFormatArgs()...)
	args = append(args, "-i", device, "-t", "2", "-f", "null", "-")
	return exec.Command(ffmpegPath, args...).Run()
}
