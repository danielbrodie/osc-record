package capture

import "fmt"

type AVFoundationMode struct{}

func (AVFoundationMode) Name() string {
	return ModeAVFoundation
}

func (AVFoundationMode) Summary() string {
	return "avfoundation (manual format)"
}

func (AVFoundationMode) BuildInputArgs(videoDevice, audioDevice string) []string {
	return []string{"-f", "avfoundation", "-framerate", "29.97", "-pixel_format", "uyvy422", "-i", fmt.Sprintf("%s:%s", videoDevice, audioDevice)}
}

func (AVFoundationMode) NeedsAudio() bool {
	return true
}

func (AVFoundationMode) SignalProbe(ffmpegPath, device string) error {
	return nil
}
