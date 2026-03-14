package recorder

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/capture"
	"github.com/danielbrodie/osc-record/internal/platform"
)

type ExitStatus struct {
	Code     int
	Filename string
	Path     string
}

type ExitInfo = ExitStatus

type Recorder struct {
	ffmpegPath      string
	stopper         platform.Stopper
	mu              sync.Mutex
	current         *recording
	unexpectedExitC chan ExitStatus
}

type Slate struct {
	Show  string
	Scene string
	Take  string
}

type recording struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	filename      string
	path          string
	stopRequested bool
	done          chan ExitStatus
}

func New(ffmpegPath string, stopper platform.Stopper) *Recorder {
	return &Recorder{
		ffmpegPath:      ffmpegPath,
		stopper:         stopper,
		unexpectedExitC: make(chan ExitStatus, 1),
	}
}

func (r *Recorder) UnexpectedExit() <-chan ExitStatus {
	return r.unexpectedExitC
}

func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current != nil
}

func (r *Recorder) Start(mode capture.CaptureMode, profile, videoDevice, audioDevice, prefix, outDir string, slate Slate, verbose bool) (string, error) {
	return r.StartAt(time.Now(), mode, profile, videoDevice, audioDevice, prefix, outDir, slate, "", verbose)
}

func (r *Recorder) StartAt(startedAt time.Time, mode capture.CaptureMode, profile, videoDevice, audioDevice, prefix, outDir string, slate Slate, fileLabel string, verbose bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current != nil {
		return "", fmt.Errorf("recording already active")
	}

	filename := buildFilenameAt(prefix, profile, slate, fileLabel, startedAt)
	fullPath := filepath.Join(outDir, filename)

	args, err := buildArgs(mode, profile, videoDevice, audioDevice, fullPath, slate)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(r.ffmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	if verbose {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	rec := &recording{
		cmd:      cmd,
		stdin:    stdin,
		filename: filename,
		path:     fullPath,
		done:     make(chan ExitStatus, 1),
	}
	r.current = rec

	go r.wait(rec)
	return filename, nil
}

func (r *Recorder) StopAndWait(ctx context.Context) (ExitStatus, error) {
	r.mu.Lock()
	rec := r.current
	if rec == nil {
		r.mu.Unlock()
		return ExitStatus{}, fmt.Errorf("not recording")
	}
	rec.stopRequested = true
	r.mu.Unlock()

	if err := r.stopper.Stop(rec.cmd, rec.stdin); err != nil {
		return ExitStatus{}, err
	}

	select {
	case status := <-rec.done:
		return status, nil
	case <-ctx.Done():
		return ExitStatus{}, ctx.Err()
	}
}

func ValidProfile(profile string) bool {
	switch profile {
	case "prores", "h264", "hevc":
		return true
	default:
		return false
	}
}

func buildArgs(mode capture.CaptureMode, profile, videoDevice, audioDevice, output string, slate Slate) ([]string, error) {
	if !ValidProfile(profile) {
		return nil, fmt.Errorf("invalid profile %q", profile)
	}

	args := append([]string{}, mode.BuildInputArgs(videoDevice, audioDevice)...)
	switch profile {
	case "prores":
		args = append(args, "-c:v", "prores_ks", "-profile:v", "1", "-c:a", "pcm_s16le")
	case "h264":
		args = append(args, "-c:v", "libx264", "-crf", "18", "-preset", "fast", "-c:a", "aac", "-b:a", "192k")
	case "hevc":
		args = append(args, "-c:v", "libx265", "-crf", "22", "-preset", "fast", "-c:a", "aac", "-b:a", "192k")
	}
	if slate.Show != "" {
		args = append(args, "-metadata", "show="+slate.Show)
	}
	if slate.Scene != "" {
		args = append(args, "-metadata", "scene="+slate.Scene)
	}
	if slate.Take != "" {
		args = append(args, "-metadata", "take="+slate.Take)
	}
	args = append(args, output)
	return args, nil
}

func buildFilenameAt(prefix, profile string, slate Slate, fileLabel string, startedAt time.Time) string {
	label := sanitizePrefix(fileLabel)
	labelPart := ""
	if label != "" {
		labelPart = "-" + label
	}

	if slate.Show != "" && slate.Scene != "" && slate.Take != "" {
		return fmt.Sprintf(
			"%s-%s-%s%s-%s%s",
			sanitizePrefix(slate.Show),
			sanitizePrefix(slate.Scene),
			sanitizePrefix(slate.Take),
			labelPart,
			startedAt.Format("2006-01-02-150405"),
			extensionForProfile(profile),
		)
	}

	safePrefix := sanitizePrefix(prefix)
	if safePrefix == "" {
		safePrefix = "recording"
	}
	return fmt.Sprintf("%s%s-%s%s", safePrefix, labelPart, startedAt.Format("2006-01-02-150405"), extensionForProfile(profile))
}

func extensionForProfile(profile string) string {
	if profile == "prores" {
		return ".mov"
	}
	return ".mp4"
}

func sanitizePrefix(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return -1
		default:
			return r
		}
	}, value)
}

func (r *Recorder) wait(rec *recording) {
	_ = rec.cmd.Wait()
	code := 0
	if rec.cmd.ProcessState != nil {
		code = rec.cmd.ProcessState.ExitCode()
	}

	status := ExitStatus{
		Code:     code,
		Filename: rec.filename,
		Path:     rec.path,
	}

	r.mu.Lock()
	if r.current == rec {
		r.current = nil
	}
	stopRequested := rec.stopRequested
	r.mu.Unlock()

	rec.done <- status
	if !stopRequested {
		select {
		case r.unexpectedExitC <- status:
		default:
		}
	}
}
