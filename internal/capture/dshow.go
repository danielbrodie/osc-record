package capture

import "fmt"

type DShowMode struct{}

func (DShowMode) Name() string {
	return ModeDShow
}

func (DShowMode) Summary() string {
	return "dshow (manual format)"
}

func (DShowMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	return []string{"-f", "dshow", "-i", fmt.Sprintf("video=%q:audio=%q", videoDevice, audioDevice)}
}

func (DShowMode) NeedsAudio() bool {
	return true
}

func (DShowMode) SignalProbe(ffmpegPath, device string) error {
	return nil
}
