package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Server struct {
	progress *Progress
	trigger  chan struct{}
	started  time.Time
	source   string
}

func newServer(progress *Progress, trigger chan struct{}, sourceURI string) *Server {
	return &Server{
		progress: progress,
		trigger:  trigger,
		started:  time.Now(),
		source:   sourceURI,
	}
}

func (s *Server) Serve(ctx context.Context, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/reindex", s.handleReindex)

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	fmt.Printf("  daemon API listening on 127.0.0.1:%d\n", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.progress.snapshot()
	json.NewEncoder(w).Encode(map[string]any{
		"status":     statusString(snap.Running),
		"source":     s.source,
		"started_at": s.started,
		"reindex":    snap,
	})
}

func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case s.trigger <- struct{}{}:
		json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
	default:
		json.NewEncoder(w).Encode(map[string]string{"status": "already running"})
	}
}

func statusString(running bool) string {
	if running {
		return "reconciling"
	}
	return "idle"
}
