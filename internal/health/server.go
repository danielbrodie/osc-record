package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/danielbrodie/osc-record/internal/tui"
)

type ClipInfo = tui.ClipInfo

type StatusSnapshot struct {
	State            string `json:"state"`
	File             string `json:"file"`
	DurationSec      int    `json:"duration_sec"`
	SizeBytes        int64  `json:"size_bytes"`
	DiskFreeBytes    uint64 `json:"disk_free_bytes"`
	Device           string `json:"device"`
	SignalLocked     bool   `json:"signal_locked"`
	Format           string `json:"format"`
	ClipsThisSession int    `json:"clips_this_session"`
	OSCPort          int    `json:"osc_port"`
	RecordAddress    string `json:"record_address"`
	StopAddress      string `json:"stop_address"`
}

type Server struct {
	mu       sync.Mutex
	server   *http.Server
	clipsFn  func() []ClipInfo
}

func (s *Server) Start(port int, bind string, state func() StatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, state())
	})
	mux.HandleFunc("/clips", func(w http.ResponseWriter, r *http.Request) {
		clips := []ClipInfo{}
		if s.clipsFn != nil {
			clips = s.clipsFn()
		}
		writeJSON(w, http.StatusOK, clips)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		snapshot := state()
		if snapshot.State == "ERROR" {
			http.Error(w, "error", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", bind, port),
		Handler: mux,
	}

	go func(server *http.Server) {
		_ = server.ListenAndServe()
	}(s.server)

	return nil
}

func (s *Server) SetClipsFunc(fn func() []ClipInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clipsFn = fn
}

func (s *Server) Stop() error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()

	if server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
