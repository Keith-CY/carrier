package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/config"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/logging"
	"carrier/daemon/internal/pairing"
)

const shutdownTimeout = 30 * time.Second

func main() {
	logger := logging.Init()
	logger.Info("initializing agentd")

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var opts []lifecycle.Option

	window, err := cfg.CrashWindowDuration()
	if err != nil {
		log.Fatalf("invalid crash window %q: %v", cfg.Lifecycle.CrashWindow, err)
	}
	cooldown, err := cfg.CrashCooldownDuration()
	if err != nil {
		log.Fatalf("invalid crash cooldown %q: %v", cfg.Lifecycle.CrashCooldown, err)
	}
	opts = append(opts, lifecycle.WithCrashLoopConfig(cfg.Lifecycle.CrashThreshold, window, cooldown))

	svc := lifecycle.NewService(baseagent.NoopTriager{}, opts...)

	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		log.Fatalf("register openclaw manifest: %v", err)
	}

	logger.Info("agentd scaffold booted")
	fmt.Printf("agentd scaffold booted (listen=%s:%d log=%s/%s)\n",
		cfg.Server.Host, cfg.Server.Port, cfg.Log.Level, cfg.Log.Format)
	fmt.Println("catalog:")
	for _, entry := range catalog.DefaultEntries() {
		fmt.Printf("- %s (%s): %s\n", entry.Name, entry.ID, entry.Status)
	}

	// Generate pairing code
	pairStore, err := pairing.NewStore()
	if err != nil {
		log.Fatalf("pairing store: %v", err)
	}
	pairCode, err := pairStore.Code()
	if err != nil {
		log.Fatalf("pairing code: %v", err)
	}
	fmt.Println("")
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Printf("║  PAIRING CODE:  %s            ║\n", pairCode)
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println("")

	// Build HTTP server
	mux := buildHTTPMux(svc, pairStore)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		fmt.Printf("HTTP API listening on %s\n", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Block until signal received
	<-ctx.Done()
	fmt.Println("shutdown signal received, stopping agents...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "http server shutdown error: %v\n", err)
	}

	if err := shutdownAgents(svc, shutdownTimeout); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	fmt.Println("agentd stopped gracefully")
}

func buildHTTPMux(svc *lifecycle.Service, pairStore *pairing.Store) *http.ServeMux {
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Pairing code
	mux.HandleFunc("/api/pair-code", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		code, err := pairStore.Code()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to get pairing code")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"code": code})
	})

	// API routes
	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		agents := svc.ListAgents()
		writeJSON(w, http.StatusOK, agents)
	})

	mux.HandleFunc("/api/install", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := svc.Install(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
	})

	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := svc.Start(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
	})

	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := svc.Stop(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	})

	mux.HandleFunc("/api/status/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		agentID := strings.TrimPrefix(r.URL.Path, "/api/status/")
		if agentID == "" {
			writeJSONError(w, http.StatusBadRequest, "missing agentId in path")
			return
		}
		state, err := svc.Status(agentID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})

	mux.HandleFunc("/api/logs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		agentID := strings.TrimPrefix(r.URL.Path, "/api/logs/")
		if agentID == "" {
			writeJSONError(w, http.StatusBadRequest, "missing agentId in path")
			return
		}
		tail := 200
		if t := r.URL.Query().Get("tail"); t != "" {
			if n, err := strconv.Atoi(t); err == nil && n > 0 {
				tail = n
			}
		}
		lines, err := svc.Logs(agentID, tail)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"lines": lines})
	})

	mux.HandleFunc("/api/upgrade", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := svc.Upgrade(r.Context(), body.AgentID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/api/diagnose", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		artifactRef, err := svc.Diagnose(body.AgentID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"artifactRef": artifactRef})
	})

	return mux
}

type agentIDBody struct {
	AgentID string `json:"agentId"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeServiceError(w http.ResponseWriter, err error) {
	// Map known errors to HTTP status codes
	switch err {
	case lifecycle.ErrAgentNotFound:
		writeJSONError(w, http.StatusNotFound, err.Error())
	case lifecycle.ErrNotInstalled:
		writeJSONError(w, http.StatusConflict, err.Error())
	case lifecycle.ErrAlreadyRunning:
		writeJSONError(w, http.StatusConflict, err.Error())
	case lifecycle.ErrAlreadyStopped:
		writeJSONError(w, http.StatusConflict, err.Error())
	case lifecycle.ErrCrashLoop:
		writeJSONError(w, http.StatusConflict, err.Error())
	case lifecycle.ErrAgentRunning:
		writeJSONError(w, http.StatusConflict, err.Error())
	case lifecycle.ErrUpgradeNotSupported:
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func shutdownAgents(svc *lifecycle.Service, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- stopAllAgents(svc)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "shutdown timed out after", timeout)
		return ctx.Err()
	}
}

func stopAllAgents(svc *lifecycle.Service) error {
	agents := svc.ListAgents()
	var firstErr error
	for _, agent := range agents {
		if agent.Runtime == lifecycle.RuntimeStateRunning {
			if err := svc.Stop(context.Background(), agent.ID); err != nil {
				fmt.Fprintf(os.Stderr, "failed to stop agent %s: %v\n", agent.ID, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}
