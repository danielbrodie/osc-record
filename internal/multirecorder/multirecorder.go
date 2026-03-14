package multirecorder

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

type StartResult struct {
	Device    DeviceInfo
	Filename  string
	Path      string
	StartedAt time.Time
}

type StopResult struct {
	Device    DeviceInfo
	Exit      recorder.ExitStatus
	StartedAt time.Time
	Duration  time.Duration
}

type UnexpectedExit struct {
	Device DeviceInfo
	Exit   recorder.ExitStatus
}

type Recorder struct {
	ffmpegPath      string
	stopper         platform.Stopper
	mu              sync.Mutex
	active          []*activeRecorder
	stopWatchers    context.CancelFunc
	unexpectedExitC chan UnexpectedExit
}

type activeRecorder struct {
	device    DeviceInfo
	recorder  *recorder.Recorder
	filename  string
	path      string
	startedAt time.Time
}

func New(ffmpegPath string, stopper platform.Stopper) *Recorder {
	return &Recorder{
		ffmpegPath:      ffmpegPath,
		stopper:         stopper,
		unexpectedExitC: make(chan UnexpectedExit, 8),
	}
}

func (r *Recorder) UnexpectedExit() <-chan UnexpectedExit {
	return r.unexpectedExitC
}

func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.active) > 0
}

func (r *Recorder) Start(devices []DeviceInfo, profile, prefix, outDir string, slate recorder.Slate, verbose bool) ([]StartResult, error) {
	r.mu.Lock()
	if len(r.active) > 0 {
		r.mu.Unlock()
		return nil, fmt.Errorf("recording already active")
	}
	r.mu.Unlock()

	startedAt := time.Now()
	entries := make([]*activeRecorder, len(devices))
	results := make([]StartResult, len(devices))
	errs := make([]error, len(devices))

	var wg sync.WaitGroup
	for i, device := range devices {
		entry := &activeRecorder{
			device:    device,
			recorder:  recorder.New(r.ffmpegPath, r.stopper),
			startedAt: startedAt,
		}
		entries[i] = entry

		wg.Add(1)
		go func(index int, current *activeRecorder) {
			defer wg.Done()

			filename, err := current.recorder.StartAt(
				startedAt,
				current.device.Mode,
				profile,
				current.device.VideoDevice,
				current.device.AudioDevice,
				prefix,
				outDir,
				slate,
				current.device.Name,
				verbose,
			)
			if err != nil {
				errs[index] = fmt.Errorf("%s: %w", current.device.Name, err)
				return
			}

			current.filename = filename
			current.path = filepath.Join(outDir, filename)
			results[index] = StartResult{
				Device:    current.device,
				Filename:  filename,
				Path:      current.path,
				StartedAt: startedAt,
			}
		}(i, entry)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, errors.Join(err, r.stopStarted(entries))
	}

	watchCtx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.active = entries
	r.stopWatchers = cancel
	r.mu.Unlock()

	for _, entry := range entries {
		go r.watchUnexpectedExit(watchCtx, entry)
	}

	return results, nil
}

func (r *Recorder) Stop() ([]StopResult, error) {
	r.mu.Lock()
	if len(r.active) == 0 {
		r.mu.Unlock()
		return nil, fmt.Errorf("not recording")
	}

	entries := append([]*activeRecorder(nil), r.active...)
	r.active = nil
	cancel := r.stopWatchers
	r.stopWatchers = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	results := make([]StopResult, len(entries))
	errs := make([]error, len(entries))

	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(index int, current *activeRecorder) {
			defer wg.Done()

			exit, err := current.recorder.StopAndWait(context.Background())
			if err != nil {
				errs[index] = fmt.Errorf("%s: %w", current.device.Name, err)
				return
			}

			results[index] = StopResult{
				Device:    current.device,
				Exit:      exit,
				StartedAt: current.startedAt,
				Duration:  time.Since(current.startedAt),
			}
		}(i, entry)
	}
	wg.Wait()

	return results, errors.Join(errs...)
}

func (r *Recorder) stopStarted(entries []*activeRecorder) error {
	errs := make([]error, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || !entry.recorder.IsRecording() {
			continue
		}
		if _, err := entry.recorder.StopAndWait(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.device.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Recorder) watchUnexpectedExit(ctx context.Context, entry *activeRecorder) {
	select {
	case exit := <-entry.recorder.UnexpectedExit():
		r.removeActive(entry.recorder)
		select {
		case r.unexpectedExitC <- UnexpectedExit{Device: entry.device, Exit: exit}:
		default:
		}
	case <-ctx.Done():
	}
}

func (r *Recorder) removeActive(target *recorder.Recorder) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.active[:0]
	for _, entry := range r.active {
		if entry.recorder == target {
			continue
		}
		filtered = append(filtered, entry)
	}
	r.active = filtered
	if len(r.active) == 0 {
		r.stopWatchers = nil
	}
}
