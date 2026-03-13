package capture

import "os/exec"

type DecklinkMode struct{}

func (DecklinkMode) Name() string {
	return ModeDecklink
}

func (DecklinkMode) Summary() string {
	return "decklink (auto-detect)"
}

func (DecklinkMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	return []string{"-f", "decklink", "-i", videoDevice}
}

func (DecklinkMode) NeedsAudio() bool {
	return false
}

func (DecklinkMode) SignalProbe(ffmpegPath, device string) error {
	return exec.Command(ffmpegPath, "-f", "decklink", "-i", device, "-t", "2", "-f", "null", "-").Run()
}
