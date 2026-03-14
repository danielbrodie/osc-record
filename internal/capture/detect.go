package capture

import (
	"fmt"

	"github.com/danielbrodie/osc-record/internal/devices"
)

func ResolveMode(requested, ffmpegPath, goos, formatCode, videoInput string) (CaptureMode, string, error) {
	switch requested {
	case "", ModeAuto:
		supported, err := devices.HasDecklinkSupport(ffmpegPath)
		if err != nil {
			return nil, "", err
		}
		if supported {
			return DecklinkMode{FormatCode: formatCode, VideoInput: videoInput}, "", nil
		}
		fallback := fallbackMode(goos)
		return newMode(fallback), warningForFallback(fallback), nil
	case ModeDecklink:
		supported, err := devices.HasDecklinkSupport(ffmpegPath)
		if err != nil {
			return nil, "", err
		}
		if !supported {
			return nil, "", fmt.Errorf("Error: Capture mode set to %q but ffmpeg was not compiled with decklink support. Install ffmpeg with --with-decklink or set capture_mode to %q.", ModeDecklink, ModeAuto)
		}
		return DecklinkMode{FormatCode: formatCode, VideoInput: videoInput}, "", nil
	case ModeAVFoundation, ModeDShow:
		if !modeSupportedOnOS(requested, goos) {
			return nil, "", fmt.Errorf("Error: Capture mode %q is not supported on %s.", requested, goos)
		}
		return newMode(requested), "", nil
	default:
		return nil, "", fmt.Errorf("Error: Invalid capture mode %q.", requested)
	}
}

func newMode(name string) CaptureMode {
	switch name {
	case ModeDecklink:
		return DecklinkMode{}
	case ModeAVFoundation:
		return AVFoundationMode{}
	case ModeDShow:
		return DShowMode{}
	default:
		return nil
	}
}

func fallbackMode(goos string) string {
	if goos == "windows" {
		return ModeDShow
	}
	return ModeAVFoundation
}

func modeSupportedOnOS(mode, goos string) bool {
	switch mode {
	case ModeDecklink:
		return true
	case ModeAVFoundation:
		return goos == "darwin"
	case ModeDShow:
		return goos == "windows"
	default:
		return false
	}
}

func warningForFallback(mode string) string {
	switch mode {
	case ModeAVFoundation:
		return "Warning: ffmpeg does not support decklink input format. Falling back to avfoundation. For auto-detect signal support, install ffmpeg with --with-decklink."
	case ModeDShow:
		return "Warning: ffmpeg does not support decklink input format. Falling back to dshow. For auto-detect signal support, install ffmpeg with --with-decklink."
	default:
		return ""
	}
}
