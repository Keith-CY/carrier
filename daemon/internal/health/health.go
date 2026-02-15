// Package health provides HTTP health check endpoints for the carrier daemon.
package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// Version can be set at build time via -ldflags.
var Version = "dev"

// AgentCounter returns the number of currently running agents.
type AgentCounter interface {
	RunningAgentsCount() int
}

// Server exposes /healthz and /readyz endpoints.
type Server struct {
	startTime time.Time
	ready     atomic.Bool
	agents    AgentCounter
}

// NewServer creates a new health check server.
func NewServer(agents AgentCounter) *Server {
	return &Server{
		startTime: time.Now(),
		agents:    agents,
	}
}

// SetReady marks the daemon as fully initialized.
func (s *Server) SetReady(v bool) {
	s.ready.Store(v)
}

// Handler returns an http.ServeMux with /healthz and /readyz registered.
func (s *Server) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	return mux
}

type healthzResponse struct {
	Status        string `json:"status"`
	Uptime        string `json:"uptime"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	RunningAgents int    `json:"running_agents"`
	Version       string `json:"version"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
