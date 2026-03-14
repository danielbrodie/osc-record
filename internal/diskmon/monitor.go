package diskmon

import (
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

type Monitor struct {
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool
}

func (m *Monitor) Start(path string, send func(tui.DiskStatMsg)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}
	m.running = true
	m.stopCh = make(chan struct{})

	m.wg.Add(1)
	go m.run(path, send)
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	stopCh := m.stopCh
	m.running = false
	m.mu.Unlock()

	close(stopCh)
	m.wg.Wait()
}

func (m *Monitor) run(path string, send func(tui.DiskStatMsg)) {
	defer m.wg.Done()

	sendDiskStat(path, send)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			sendDiskStat(path, send)
		}
	}
}
