package sigpoll

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/capture"
	"github.com/danielbrodie/osc-record/internal/tui"
)

var (
	resolutionPattern = regexp.MustCompile(`\b\d{3,5}x\d{3,5}\b`)
	fpsPattern        = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\s*fps\b`)
)

type Poller struct {
	mode       string
	stopCh     chan struct{}
	triggerCh  chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	suspended  bool
	running    bool
	probeMu    sync.Mutex // held while a probe is in progress
}

func New(mode string) *Poller {
	return &Poller{mode: mode}
}

func (p *Poller) Start(device, ffmpegPath, formatCode string, send func(tui.SignalStateMsg)) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.triggerCh = make(chan struct{}, 1)

	p.wg.Add(1)
	go p.run(device, ffmpegPath, formatCode, send)
}

func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	stopCh := p.stopCh
	p.running = false
	p.mu.Unlock()

	close(stopCh)
	p.wg.Wait()
}

// Suspend pauses the poller and waits for any in-progress probe to finish.
// Safe to call before opening the capture device in recording mode.
func (p *Poller) Suspend() {
	p.mu.Lock()
	p.suspended = true
	p.mu.Unlock()
	// Wait for any running probe to complete before returning.
	// This ensures the device is released before the caller opens it.
	p.probeMu.Lock()
	p.probeMu.Unlock() //nolint:staticcheck
}

func (p *Poller) Resume() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.suspended = false
	triggerCh := p.triggerCh
	p.mu.Unlock()

	select {
	case triggerCh <- struct{}{}:
	default:
	}
}

func (p *Poller) run(device, ffmpegPath, formatCode string, send func(tui.SignalStateMsg)) {
	defer p.wg.Done()

	if p.mode == capture.ModeAVFoundation || p.mode == capture.ModeDShow {
		if send != nil {
			send(tui.SignalStateMsg{Device: device, Locked: false, Input: ""})
		}
		<-p.stopCh
		return
	}

	p.probe(device, ffmpegPath, formatCode, send)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-p.triggerCh:
			if p.isSuspended() {
				continue
			}
			p.probe(device, ffmpegPath, formatCode, send)
		case <-ticker.C:
			if p.isSuspended() {
				continue
			}
			p.probe(device, ffmpegPath, formatCode, send)
		}
	}
}

func (p *Poller) isSuspended() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.suspended
}

func (p *Poller) probe(device, ffmpegPath, formatCode string, send func(tui.SignalStateMsg)) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()
	if send == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	args := []string{"-hide_banner", "-f", "decklink"}
	if formatCode != "" {
		args = append(args, "-format_code", formatCode)
	}
	args = append(args, "-i", device, "-t", "1", "-f", "null", "-")

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	err := cmd.Run()

	output := stderr.String()
	msg := tui.SignalStateMsg{
		Device:     device,
		Input:      "SDI",
		Format:     formatCode,
		Resolution: firstMatch(output, resolutionPattern),
		FPS:        firstSubmatch(output, fpsPattern),
		Locked:     err == nil,
	}
	if err != nil {
		msg.Err = probeError(output, err)
	}
	send(msg)
}

func firstMatch(value string, pattern *regexp.Regexp) string {
	return pattern.FindString(value)
}

func firstSubmatch(value string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(value)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func probeError(output string, err error) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return line
	}
	return err.Error()
}
