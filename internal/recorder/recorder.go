package recorder

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/brodiegraphics/osc-record/internal/capture"
	"github.com/brodiegraphics/osc-record/internal/platform"
)

var unsafeChars = regexp.MustCompile(`[/\\:*?"<>|]`)

func SanitizePrefix(prefix string) string {
	return unsafeChars.ReplaceAllString(prefix, "")
}

func OutputFilename(prefix, profile string) string {
	ext := "mp4"
	if strings.ToLower(profile) == "prores" {
		ext = "mov"
	}
	ts := time.Now().Format("2006-01-02-150405")
	return fmt.Sprintf("%s-%s.%s", SanitizePrefix(prefix), ts, ext)
}

type Recorder struct {
	FFmpegPath  string
	OutputDir   string
	Prefix      string
	Profile     string
	Mode        capture.Mode
	Stopper     platform.Stopper

	cmd         *exec.Cmd
	currentFile string
}

func (r *Recorder) IsRecording() bool {
	return r.cmd != nil
}

func (r *Recorder) Start() (string, error) {
	filename := OutputFilename(r.Prefix, r.Profile)
	outputPath := filepath.Join(r.OutputDir, filename)

	args := r.Mode.InputArgs()
	args = append(args, "-i", r.Mode.InputDevice())
	args = append(args, r.profileArgs(outputPath)...)

	r.cmd = exec.Command(r.FFmpegPath, args...)
	if err := r.cmd.Start(); err != nil {
		r.cmd = nil
		return "", err
	}
	r.currentFile = filename
	return filename, nil
}

func (r *Recorder) Stop() (string, error) {
	if r.cmd == nil {
		return "", nil
	}
	if err := r.Stopper.Stop(r.cmd); err != nil {
		return "", err
	}
	_ = r.cmd.Wait()
	f := r.currentFile
	r.cmd = nil
	r.currentFile = ""
	return f, nil
}

// WatchExit monitors ffmpeg in a goroutine and calls onExit if it exits unexpectedly.
// The caller must only invoke this while holding any relevant mutex, and onExit
// will be called without holding it.
func (r *Recorder) WatchExit(onExit func(code int)) {
	cmd := r.cmd
	go func() {
		if cmd == nil {
			return
		}
		state, _ := cmd.Process.Wait()
		code := 0
		if state != nil {
			code = state.ExitCode()
		}
		onExit(code)
	}()
}

func (r *Recorder) profileArgs(outputPath string) []string {
	switch strings.ToLower(r.Profile) {
	case "prores":
		return []string{"-c:v", "prores_ks", "-profile:v", "1", "-c:a", "pcm_s16le", outputPath}
	case "hevc":
		return []string{"-c:v", "libx265", "-crf", "22", "-preset", "fast", "-c:a", "aac", "-b:a", "192k", outputPath}
	default: // h264
		return []string{"-c:v", "libx264", "-crf", "18", "-preset", "fast", "-c:a", "aac", "-b:a", "192k", outputPath}
	}
}
