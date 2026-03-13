//go:build !darwin && !windows

package capture

import "fmt"

type AVFoundationMode struct {
	VideoDevice string
	AudioDevice string
}

func (a *AVFoundationMode) InputArgs() []string {
	return []string{"-f", "avfoundation", "-framerate", "29.97", "-pixel_format", "uyvy422"}
}

func (a *AVFoundationMode) InputDevice() string {
	return fmt.Sprintf("%s:%s", a.VideoDevice, a.AudioDevice)
}

func (a *AVFoundationMode) Name() string {
	return "avfoundation (manual format)"
}
