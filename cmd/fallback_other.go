//go:build !darwin && !windows

package cmd

import "github.com/brodiegraphics/osc-record/internal/capture"

func buildFallbackMode(videoDevice, audioDevice string) capture.Mode {
	return &capture.AVFoundationMode{VideoDevice: videoDevice, AudioDevice: audioDevice}
}
