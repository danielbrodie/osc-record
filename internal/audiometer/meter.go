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

// ametadata output format: "lavfi.astats.Overall.RMS_level=-23.45" or "-inf"
var (
	rmsRe = regexp.MustCompile(`lavfi\.astats\.Overall\.RMS_level=(-[\d.]+|-inf|inf)`)
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

// Stop terminates the audio meter process and waits for it to exit.
func (m *Meter) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.running = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
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

func (m *Meter) probe(ctx context.Context, ffmpegPath string, inputArgs []string, send func(tui.AudioLevelMsg)) {
	// Build the ffmpeg command: read device, run astats every 0.5s, output to null
	// astats with metadata=1 emits per-channel RMS to stderr.
	args := append(inputArgs,
		"-vn",                  // no video decoding
		"-af", "astats=metadata=1:reset=1,ametadata=print:key=lavfi.astats.Overall.RMS_level:file=-",
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

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			return
		default:
		}

		line := scanner.Text()
		// ametadata output per frame: "lavfi.astats.Overall.RMS_level=-23.45"
		if matches := rmsRe.FindStringSubmatch(line); len(matches) > 1 {
			v := matches[1]
			var db float64
			if v == "inf" || v == "-inf" {
				db = -144
			} else {
				db, _ = strconv.ParseFloat(v, 64)
				if db < -144 {
					db = -144
				}
			}
			// Overall level sent to both channels (stereo split needs per-channel astats keys)
			send(tui.AudioLevelMsg{Left: db, Right: db})
		}
	}

	cmd.Wait()
}
