//go:build windows

package capture

func NewFallbackMode(videoDevice, audioDevice string) Mode {
	return &DShowMode{VideoDevice: videoDevice, AudioDevice: audioDevice}
}
