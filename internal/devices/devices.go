package devices

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const (
	ModeDecklink     = "decklink"
	ModeAVFoundation = "avfoundation"
	ModeDShow        = "dshow"
)

var errUnsupportedInput = errors.New("unsupported ffmpeg input format")

type Device struct {
	ID   string
	Name string
}

func (d Device) ConfigValue() string {
	if d.ID != "" {
		return d.ID
	}
	return d.Name
}

type Group struct {
	Mode  string
	Video []Device
	Audio []Device
}

func (g Group) ModeDescription() string {
	switch g.Mode {
	case ModeDecklink:
		return "decklink (auto-detect signal format)"
	case ModeAVFoundation:
		return "avfoundation (manual format required)"
	case ModeDShow:
		return "dshow (manual format required)"
	default:
		return g.Mode
	}
}

func ProbeForPlatform(ffmpegPath, goos string) ([]Group, error) {
	groups := make([]Group, 0, 2)

	if group, err := ProbeMode(ffmpegPath, ModeDecklink); err == nil {
		groups = append(groups, group)
	} else if !errors.Is(err, errUnsupportedInput) {
		return nil, err
	}

	group, err := ProbeMode(ffmpegPath, fallbackMode(goos))
	if err != nil {
		return nil, err
	}
	groups = append(groups, group)
	return groups, nil
}

func ProbeMode(ffmpegPath, mode string) (Group, error) {
	switch mode {
	case ModeDecklink:
		output, err := runListCommand(ffmpegPath, []string{"-hide_banner", "-f", "decklink", "-list_devices", "true", "-i", ""})
		if strings.Contains(output, "Unknown input format") {
			return Group{}, errUnsupportedInput
		}
		if err != nil && len(strings.TrimSpace(output)) == 0 {
			return Group{}, err
		}
		return Group{Mode: ModeDecklink, Video: parseDecklink(output)}, nil
	case ModeAVFoundation:
		output, err := runListCommand(ffmpegPath, []string{"-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", ""})
		if err != nil && len(strings.TrimSpace(output)) == 0 {
			return Group{}, err
		}
		video, audio := parseAVFoundation(output)
		return Group{Mode: ModeAVFoundation, Video: video, Audio: audio}, nil
	case ModeDShow:
		output, err := runListCommand(ffmpegPath, []string{"-hide_banner", "-f", "dshow", "-list_devices", "true", "-i", "dummy"})
		if err != nil && len(strings.TrimSpace(output)) == 0 {
			return Group{}, err
		}
		video, audio := parseDShow(output)
		return Group{Mode: ModeDShow, Video: video, Audio: audio}, nil
	default:
		return Group{}, fmt.Errorf("unsupported capture mode %q", mode)
	}
}

func HasDecklinkSupport(ffmpegPath string) (bool, error) {
	output, err := runListCommand(ffmpegPath, []string{"-hide_banner", "-f", "decklink", "-list_devices", "true", "-i", ""})
	if strings.Contains(output, "Unknown input format") {
		return false, nil
	}
	if err != nil && len(strings.TrimSpace(output)) == 0 {
		return false, err
	}
	return true, nil
}

func MatchDevice(items []Device, value string) (Device, error) {
	for _, item := range items {
		if item.ID != "" && item.ID == value {
			return item, nil
		}
		if strings.EqualFold(item.Name, value) {
			return item, nil
		}
	}
	return Device{}, errors.New("device not found")
}

func fallbackMode(goos string) string {
	if goos == "windows" {
		return ModeDShow
	}
	return ModeAVFoundation
}

func runListCommand(ffmpegPath string, args []string) (string, error) {
	cmd := exec.Command(ffmpegPath, args...)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err := cmd.Run()
	return buffer.String(), err
}

func parseDecklink(output string) []Device {
	quotePattern := regexp.MustCompile(`['"]([^'"]+)['"]`)
	lines := strings.Split(output, "\n")
	items := make([]Device, 0)
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "deprecated") ||
			strings.Contains(lower, "blackmagic decklink devices") ||
			strings.Contains(lower, "could not list") {
			continue
		}
		matches := quotePattern.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		name := strings.TrimSpace(matches[1])
		if name == "" {
			continue
		}
		items = append(items, Device{Name: name})
	}
	return uniqueDevices(items)
}

func parseAVFoundation(output string) ([]Device, []Device) {
	indexPattern := regexp.MustCompile(`\[(\d+)\]\s+(.+)$`)
	lines := strings.Split(output, "\n")
	section := ""
	video := make([]Device, 0)
	audio := make([]Device, 0)

	for _, line := range lines {
		switch {
		case strings.Contains(line, "AVFoundation video devices"):
			section = "video"
		case strings.Contains(line, "AVFoundation audio devices"):
			section = "audio"
		default:
			matches := indexPattern.FindStringSubmatch(strings.TrimSpace(line))
			if len(matches) != 3 {
				continue
			}
			item := Device{ID: matches[1], Name: strings.TrimSpace(matches[2])}
			if section == "video" {
				video = append(video, item)
			} else if section == "audio" {
				audio = append(audio, item)
			}
		}
	}

	return video, audio
}

func parseDShow(output string) ([]Device, []Device) {
	quotePattern := regexp.MustCompile(`"([^"]+)"`)
	lines := strings.Split(output, "\n")
	section := ""
	video := make([]Device, 0)
	audio := make([]Device, 0)

	for _, line := range lines {
		switch {
		case strings.Contains(line, "DirectShow video devices"):
			section = "video"
		case strings.Contains(line, "DirectShow audio devices"):
			section = "audio"
		default:
			matches := quotePattern.FindStringSubmatch(line)
			if len(matches) != 2 {
				continue
			}
			item := Device{Name: strings.TrimSpace(matches[1])}
			if section == "video" {
				video = append(video, item)
			} else if section == "audio" {
				audio = append(audio, item)
			}
		}
	}

	return uniqueDevices(video), uniqueDevices(audio)
}

func uniqueDevices(items []Device) []Device {
	seen := make(map[string]struct{}, len(items))
	unique := make([]Device, 0, len(items))
	for _, item := range items {
		key := item.ID + "|" + strings.ToLower(item.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, item)
	}
	return unique
}

// BestAudioMatch finds the best audio device matching the given video device name.
// It looks for an audio device whose name contains the video device name or a
// significant word from it (case-insensitive). Returns error if no match found.
func BestAudioMatch(audioDevices []Device, videoName string) (Device, error) {
	videoLower := strings.ToLower(videoName)

	// Exact substring match first
	for _, d := range audioDevices {
		if strings.Contains(strings.ToLower(d.Name), videoLower) ||
			strings.Contains(videoLower, strings.ToLower(d.Name)) {
			return d, nil
		}
	}

	// Word-by-word match — find audio device sharing a meaningful word with video device
	videoWords := strings.Fields(videoLower)
	bestScore := 0
	var best Device
	for _, d := range audioDevices {
		audioLower := strings.ToLower(d.Name)
		score := 0
		for _, w := range videoWords {
			if len(w) > 2 && strings.Contains(audioLower, w) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = d
		}
	}
	if bestScore > 0 {
		return best, nil
	}
	return Device{}, fmt.Errorf("no audio device matched %q", videoName)
}
