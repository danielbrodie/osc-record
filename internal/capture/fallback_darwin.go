//go:build darwin

package capture

func NewFallbackMode(videoDevice, audioDevice string) Mode {
	return &AVFoundationMode{VideoDevice: videoDevice, AudioDevice: audioDevice}
}
