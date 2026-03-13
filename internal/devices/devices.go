package devices

import (
	"os/exec"
	"runtime"
	"strings"
)

type DeviceList struct {
	DecklinkDevices []string
	VideoDevices    []string
	AudioDevices    []string
	Mode            string // "decklink" or "avfoundation" or "dshow"
}

func ListDecklink(ffmpegPath string) []string {
	cmd := exec.Command(ffmpegPath, "-f", "decklink", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "Unknown input format") {
		return nil
	}
	var devices []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// ffmpeg decklink device lines start with a device name after the log prefix
		if strings.Contains(line, "decklink") && strings.Contains(line, "@") {
			// Parse: "[decklink @ 0x...] <device name>"
			if idx := strings.Index(line, "] "); idx >= 0 {
				name := strings.TrimSpace(line[idx+2:])
				if name != "" && !strings.HasPrefix(name, "decklink") {
					devices = append(devices, name)
				}
			}
		}
	}
	return devices
}

func ListAVFoundation(ffmpegPath string) (video []string, audio []string) {
	cmd := exec.Command(ffmpegPath, "-f", "avfoundation", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()
	inAudio := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "AVFoundation audio devices") {
			inAudio = true
			continue
		}
		if strings.Contains(line, "AVFoundation video devices") {
			inAudio = false
			continue
		}
		if strings.Contains(line, "[AVFoundation") && strings.Contains(line, "] [") {
			// Parse: "[AVFoundation indev @ 0x...] [0] Device Name"
			if idx := strings.LastIndex(line, "] "); idx >= 0 {
				name := strings.TrimSpace(line[idx+2:])
				if name != "" {
					if inAudio {
						audio = append(audio, name)
					} else {
						video = append(video, name)
					}
				}
			}
		}
	}
	return
}

func ListDShow(ffmpegPath string) (video []string, audio []string) {
	cmd := exec.Command(ffmpegPath, "-f", "dshow", "-list_devices", "true", "-i", "dummy")
	out, _ := cmd.CombinedOutput()
	inAudio := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "DirectShow audio devices") {
			inAudio = true
			continue
		}
		if strings.Contains(line, "DirectShow video devices") {
			inAudio = false
			continue
		}
		if strings.Contains(line, "[dshow") && strings.Contains(line, "] \"") {
			if idx := strings.Index(line, "] \""); idx >= 0 {
				name := strings.Trim(line[idx+3:], "\"")
				if name != "" {
					if inAudio {
						audio = append(audio, name)
					} else {
						video = append(video, name)
					}
				}
			}
		}
	}
	return
}

func FFmpegPath(configured string) string {
	if configured != "" {
		return configured
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	return "ffmpeg"
}

func PlatformFallback() string {
	if runtime.GOOS == "windows" {
		return "dshow"
	}
	return "avfoundation"
}
