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
	// No pixel_format or framerate constraints — let the device advertise what it supports.
	// The C920 and similar webcams don't support uyvy422 or 29.97fps.
	input := videoDevice
	if audioDevice != "" {
		input = fmt.Sprintf("%s:%s", videoDevice, audioDevice)
	}
	return []string{"-f", "avfoundation", "-i", input}
}

func (AVFoundationMode) NeedsAudio() bool {
	return true
}

func (AVFoundationMode) SignalProbe(ffmpegPath, device string) error {
	return nil
}
