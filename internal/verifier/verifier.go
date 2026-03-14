package verifier

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

type Verifier struct{}

type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType string `json:"codec_type"`
}

type probeFormat struct {
	Duration string `json:"duration"`
}

func (v Verifier) Verify(file string, expectedDuration time.Duration, needsAudio bool, send func(tui.ClipVerifiedMsg)) {
	go func() {
		msg := tui.ClipVerifiedMsg{File: file, OK: true}

		ffprobePath, err := exec.LookPath("ffprobe")
		if err != nil {
			// Fall back to ffprobe-decklink (installed by the ffmpeg-decklink formula)
			ffprobePath, err = exec.LookPath("ffprobe-decklink")
		}
		if err != nil {
			msg.OK = false
			msg.Errors = []string{"ffprobe not found on PATH"}
			sendResult(send, msg)
			return
		}

		cmd := exec.Command(ffprobePath, "-v", "error", "-show_streams", "-show_format", "-of", "json", file)
		output, err := cmd.Output()
		if err != nil {
			msg.OK = false
			msg.Errors = []string{err.Error()}
			sendResult(send, msg)
			return
		}

		var probe probeOutput
		if err := json.Unmarshal(output, &probe); err != nil {
			msg.OK = false
			msg.Errors = []string{"invalid ffprobe JSON"}
			sendResult(send, msg)
			return
		}

		hasVideo := false
		hasAudio := false
		for _, stream := range probe.Streams {
			switch stream.CodecType {
			case "video":
				hasVideo = true
			case "audio":
				hasAudio = true
			}
		}

		if !hasVideo {
			msg.OK = false
			msg.Errors = append(msg.Errors, "missing video stream")
		}
		if needsAudio && !hasAudio {
			msg.OK = false
			msg.Errors = append(msg.Errors, "missing audio stream")
		}

		if probe.Format.Duration != "" && expectedDuration > 0 {
			durationSec, err := strconv.ParseFloat(probe.Format.Duration, 64)
			if err != nil {
				msg.OK = false
				msg.Errors = append(msg.Errors, "invalid duration")
			} else {
				actualDuration := time.Duration(durationSec * float64(time.Second))
				diff := actualDuration - expectedDuration
				if diff < 0 {
					diff = -diff
				}
				if diff > 2*time.Second {
					msg.OK = false
					msg.Errors = append(msg.Errors, fmt.Sprintf("duration mismatch: expected %s, got %s", expectedDuration.Round(time.Second), actualDuration.Round(time.Second)))
				}
			}
		}

		sendResult(send, msg)
	}()
}

func sendResult(send func(tui.ClipVerifiedMsg), msg tui.ClipVerifiedMsg) {
	if send == nil {
		return
	}
	send(msg)
}
