// Package audiometer runs a persistent ffmpeg process to extract audio RMS
// levels from a capture device and reports them as AudioLevelMsg.
package audiometer

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

// Per-channel ametadata output: "lavfi.astats.1.RMS_level=-23.45", "lavfi.astats.2.RMS_level=-21.10"
var (
	ch1Re = regexp.MustCompile(`lavfi\.astats\.1\.RMS_level=(-[\d.]+|-inf|inf)`)
	ch2Re = regexp.MustCompile(`lavfi\.astats\.2\.RMS_level=(-[\d.]+|-inf|inf)`)
)

// Meter runs a background ffmpeg process reading audio levels from a device.
type Meter struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

// Start launches the audio meter process. send is called with level updates.
// inputArgs should be the same args used for BuildInputArgs (e.g. -f decklink -format_code 24ps -i "Device").
// Meter automatically stops when ctx is done.
func (m *Meter) Start(ffmpegPath string, inputArgs []string, send func(tui.AudioLevelMsg)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}
	m.running = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	go m.run(ctx, ffmpegPath, inputArgs, send)
}

// Stop terminates the audio meter process and waits briefly for the process to exit.
func (m *Meter) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.running = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
		// Give the ffmpeg process a moment to actually release the device handle.
		// decklink requires exclusive access; starting recording before the process
		// exits causes "Cannot enable video input".
		time.Sleep(300 * time.Millisecond)
	}
}

// IsRunning returns true if the meter is active.
func (m *Meter) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Meter) run(ctx context.Context, ffmpegPath string, inputArgs []string, send func(tui.AudioLevelMsg)) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.probe(ctx, ffmpegPath, inputArgs, send)

		// Brief pause before retry (handles device-busy after recording stops)
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func parseDB(s string) float64 {
	if s == "inf" || s == "-inf" {
		return -144
	}
	v, _ := strconv.ParseFloat(s, 64)
	if v < -144 {
		return -144
	}
	return v
}

func (m *Meter) probe(ctx context.Context, ffmpegPath string, inputArgs []string, send func(tui.AudioLevelMsg)) {
	// Build the ffmpeg command: read device, run astats every 0.5s, output to null
	// astats with metadata=1 emits per-channel RMS to stderr.
	args := append(inputArgs,
		"-vn", // no video decoding
		"-af", "astats=metadata=1:reset=1,ametadata=print:file=-",
		"-f", "null", "-",
	)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	// astats metadata goes to stdout via ametadata print to file=-
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	var left, right float64 = -144, -144
	hasLeft, hasRight := false, false

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			return
		default:
		}

		line := scanner.Text()

		if m := ch1Re.FindStringSubmatch(line); len(m) > 1 {
			left = parseDB(m[1])
			hasLeft = true
		} else if m := ch2Re.FindStringSubmatch(line); len(m) > 1 {
			right = parseDB(m[1])
			hasRight = true
		}

		if hasLeft && hasRight {
			send(tui.AudioLevelMsg{Left: left, Right: right})
			hasLeft, hasRight = false, false
		}
	}

	cmd.Wait()
}
