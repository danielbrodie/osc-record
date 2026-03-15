package capture

import "fmt"

type DShowMode struct {
	VideoSize string
	FrameRate string
}

func (DShowMode) Name() string {
	return ModeDShow
}

func (m DShowMode) Summary() string {
	if m.VideoSize != "" {
		return fmt.Sprintf("dshow (%s @ %s fps)", m.VideoSize, m.FrameRate)
	}
	return "dshow (detecting...)"
}

func (m DShowMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	args := []string{"-f", "dshow"}
	if m.VideoSize != "" {
		args = append(args, "-video_size", m.VideoSize)
	}
	if m.FrameRate != "" {
		args = append(args, "-framerate", m.FrameRate)
	}
	input := "video=" + videoDevice
	if audioDevice != "" {
		input += ":audio=" + audioDevice
	}
	return append(args, "-i", input)
}

func (DShowMode) NeedsAudio() bool {
	return true
}

func (m DShowMode) SignalProbe(ffmpegPath, device string) error {
	return nil
}
