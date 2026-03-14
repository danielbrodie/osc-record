package multirecorder

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/capture"
	"github.com/danielbrodie/osc-record/internal/platform"
	"github.com/danielbrodie/osc-record/internal/recorder"
)

type DeviceInfo struct {
	Name        string
	Mode        capture.CaptureMode
	VideoDevice string
	AudioDevice string
}

type MultiRecorder struct {
	mu              sync.Mutex
	devices         []deviceRecorder
	active          map[string]*activeRecorder
	watchCancel     context.CancelFunc
	stopping        bool
	unexpectedExits chan recorder.ExitInfo
}

type deviceRecorder struct {
	info     DeviceInfo
	recorder *recorder.Recorder
}

type activeRecorder struct {
	device    DeviceInfo
	recorder  *recorder.Recorder
	filename  string
	path      string
	startedAt time.Time
}

// New creates one Recorder per device.
func New(ffmpegPath string, platform platform.Platform, devices []DeviceInfo) *MultiRecorder {
	entries := make([]deviceRecorder, 0, len(devices))
	for _, device := range devices {
		entries = append(entries, deviceRecorder{
			info:     device,
			recorder: recorder.New(ffmpegPath, platform),
		})
	}

	return &MultiRecorder{
		devices:         entries,
		unexpectedExits: make(chan recorder.ExitInfo, len(devices)),
		active:          make(map[string]*activeRecorder, len(devices)),
	}
}

// Start starts all recorders simultaneously. Returns an error if any start fails.
func (m *MultiRecorder) Start(mode capture.CaptureMode, profile, prefix, outDir string, slate recorder.Slate, verbose bool) ([]string, error) {
	m.mu.Lock()
	if len(m.active) > 0 || m.stopping {
		m.mu.Unlock()
		return nil, fmt.Errorf("recording already active")
	}
	devices := append([]deviceRecorder(nil), m.devices...)
	m.mu.Unlock()

	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices configured")
	}

	startedAt := time.Now()
	entries := make([]*activeRecorder, len(devices))
	filenames := make([]string, len(devices))
	errList := make([]error, len(devices))

	var wg sync.WaitGroup
	for i, device := range devices {
		entry := &activeRecorder{
			device:    device.info,
			recorder:  device.recorder,
			startedAt: startedAt,
		}
		entries[i] = entry

		wg.Add(1)
		go func(index int, current *activeRecorder) {
			defer wg.Done()

			filename, err := current.recorder.StartAt(
				startedAt,
				modeForDevice(mode, current.device),
				profile,
				current.device.VideoDevice,
				current.device.AudioDevice,
				prefix,
				outDir,
				slate,
				shortName(current.device.Name),
				verbose,
			)
			if err != nil {
				errList[index] = fmt.Errorf("%s: %w", current.device.Name, err)
				return
			}

			current.filename = filename
			current.path = filepath.Join(outDir, filename)
			filenames[index] = filename
		}(i, entry)
	}
	wg.Wait()

	if err := errors.Join(errList...); err != nil {
		return nil, errors.Join(err, m.stopStarted(entries))
	}

	watchCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	for _, entry := range entries {
		m.active[entry.filename] = entry
	}
	m.watchCancel = cancel
	m.mu.Unlock()

	for _, entry := range entries {
		go m.watchUnexpectedExit(watchCtx, entry)
	}

	return filenames, nil
}

// Stop stops all recorders and waits for them to finish.
func (m *MultiRecorder) Stop() map[string]error {
	m.mu.Lock()
	if len(m.active) == 0 {
		m.mu.Unlock()
		return map[string]error{}
	}

	entries := make([]*activeRecorder, 0, len(m.active))
	for _, entry := range m.active {
		entries = append(entries, entry)
	}
	m.stopping = true
	cancel := m.watchCancel
	m.watchCancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	results := make(map[string]error, len(entries))
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, entry := range entries {
		wg.Add(1)
		go func(current *activeRecorder) {
			defer wg.Done()
			_, err := current.recorder.StopAndWait(context.Background())
			mu.Lock()
			results[current.filename] = err
			mu.Unlock()
		}(entry)
	}
	wg.Wait()

	m.mu.Lock()
	m.active = make(map[string]*activeRecorder, len(m.devices))
	m.stopping = false
	m.mu.Unlock()

	return results
}

// IsRecording returns true if any recorder is still active.
func (m *MultiRecorder) IsRecording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active) > 0
}

// UnexpectedExits returns a channel that receives any unexpected recorder exit.
func (m *MultiRecorder) UnexpectedExits() <-chan recorder.ExitInfo {
	return m.unexpectedExits
}

func (m *MultiRecorder) stopStarted(entries []*activeRecorder) error {
	errList := make([]error, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || !entry.recorder.IsRecording() {
			continue
		}
		if _, err := entry.recorder.StopAndWait(context.Background()); err != nil {
			errList = append(errList, fmt.Errorf("%s: %w", entry.device.Name, err))
		}
	}
	return errors.Join(errList...)
}

func (m *MultiRecorder) watchUnexpectedExit(ctx context.Context, entry *activeRecorder) {
	select {
	case exit := <-entry.recorder.UnexpectedExit():
		m.removeActive(entry.filename)
		select {
		case m.unexpectedExits <- exit:
		default:
		}
	case <-ctx.Done():
	}
}

func (m *MultiRecorder) removeActive(filename string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.active, filename)
	if len(m.active) == 0 {
		m.watchCancel = nil
	}
}

func modeForDevice(defaultMode capture.CaptureMode, device DeviceInfo) capture.CaptureMode {
	if device.Mode != nil {
		return device.Mode
	}
	return defaultMode
}

func shortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
