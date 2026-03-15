package capture

type DShowMode struct{}

func (DShowMode) Name() string {
	return ModeDShow
}

func (DShowMode) Summary() string {
	return "dshow (manual format)"
}

func (DShowMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	input := "video=" + videoDevice
	if audioDevice != "" {
		input += ":audio=" + audioDevice
	}
	return []string{"-f", "dshow", "-i", input}
}

func (DShowMode) NeedsAudio() bool {
	return true
}

func (DShowMode) SignalProbe(ffmpegPath, device string) error {
	return nil
}
