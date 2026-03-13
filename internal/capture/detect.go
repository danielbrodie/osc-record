package capture

import (
	"os/exec"
	"runtime"
	"strings"
)

// SupportsDecklink returns true if ffmpeg supports the decklink input format.
func SupportsDecklink(ffmpegPath string) bool {
	cmd := exec.Command(ffmpegPath, "-f", "decklink", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()
	return !strings.Contains(string(out), "Unknown input format")
}

// ResolveMode determines the best capture mode given the config and ffmpeg path.
// It never prompts the user — mode selection is automatic (decklink wins if supported).
func ResolveMode(ffmpegPath, configuredMode string) string {
	switch configuredMode {
	case "decklink", "avfoundation", "dshow":
		return configuredMode
	default: // "auto"
		if SupportsDecklink(ffmpegPath) {
			return "decklink"
		}
		if runtime.GOOS == "windows" {
			return "dshow"
		}
		return "avfoundation"
	}
}
