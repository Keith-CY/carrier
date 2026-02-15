package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/config"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/logging"
)

const (
	shutdownTimeout = 30 * time.Second
	defaultLogsTail = 200
	maxLogsTail     = 1000
)

// agentIDPattern allows alphanumeric characters, hyphens, underscores, and dots.
var agentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

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

	// Validate security: refuse non-loopback binding without an API token.
	if cfg.Server.APIToken == "" && !isLoopback(cfg.Server.Host) {
		log.Fatalf("CARRIER_SERVER_API_TOKEN must be set when listening on non-loopback address %q", cfg.Server.Host)
	}
	if cfg.Server.APIToken == "" {
		// Force loopback-only binding when no token is configured.
		cfg.Server.Host = "127.0.0.1"
		logger.Info("no API token configured; forcing loopback-only bind (127.0.0.1)")
	}

	// Build HTTP server
	ready := &atomic.Bool{}
	ready.Store(false)
	mux := buildHTTPMux(svc, ready)
	var handler http.Handler = mux
	if cfg.Server.APIToken != "" {
		handler = bearerAuthMiddleware(cfg.Server.APIToken, mux)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		fmt.Printf("HTTP API listening on %s\n", addr)
		serverErrCh <- httpServer.ListenAndServe()
	}()
	ready.Store(true)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	select {
	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
		return
	case <-ctx.Done():
		ready.Store(false)
		fmt.Println("shutdown signal received, stopping agents...")
	}

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

func buildHTTPMux(svc *lifecycle.Service, ready *atomic.Bool) *http.ServeMux {
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready == nil || !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	register := func(path string, handler http.HandlerFunc) {
		mux.HandleFunc(path, handler)
		if strings.HasPrefix(path, "/api/") {
			mux.HandleFunc("/api/v1"+strings.TrimPrefix(path, "/api"), handler)
		}
	}

	// API routes
	register("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		agents := svc.ListAgents()
		writeJSON(w, http.StatusOK, agents)
	})

	register("/api/install", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := svc.Install(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
	})

	register("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := svc.Start(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
	})

	register("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := svc.Stop(r.Context(), body.AgentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	})

	register("/api/status/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		raw := trimPathByPrefixes(r.URL.Path, "/api/status/", "/api/v1/status/")
		agentID, err := parsePathAgentID(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		state, err := svc.Status(agentID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})

	register("/api/logs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		raw := trimPathByPrefixes(r.URL.Path, "/api/logs/", "/api/v1/logs/")
		agentID, err := parsePathAgentID(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		tail := parseLogsTail(r.URL.Query().Get("tail"))
		lines, err := svc.Logs(agentID, tail)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"lines": lines})
	})

	register("/api/upgrade", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := svc.Upgrade(r.Context(), body.AgentID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	register("/api/diagnose", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body agentIDBody
		if !decodeBody(w, r, &body) {
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
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

// validateAgentID checks that an agent ID is safe and well-formed.
func validateAgentID(id string) error {
	if id == "" {
		return fmt.Errorf("agent ID must not be empty")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("agent ID must not contain path separators")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("agent ID must not contain parent-directory tokens")
	}
	if !agentIDPattern.MatchString(id) {
		return fmt.Errorf("agent ID contains invalid characters")
	}
	return nil
}

func trimPathByPrefixes(path string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return path
}

// parsePathAgentID extracts, URL-decodes, and validates an agent ID from a URL path segment.
func parsePathAgentID(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("missing agentId in path")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL encoding in agent ID: %w", err)
	}
	if err := validateAgentID(decoded); err != nil {
		return "", err
	}
	return decoded, nil
}

func parseLogsTail(raw string) int {
	if raw == "" {
		return defaultLogsTail
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultLogsTail
	}
	if n > maxLogsTail {
		return maxLogsTail
	}
	return n
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
	switch {
	case errors.Is(err, lifecycle.ErrAgentNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, lifecycle.ErrNotInstalled):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lifecycle.ErrAlreadyRunning):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lifecycle.ErrAlreadyStopped):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lifecycle.ErrCrashLoop):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lifecycle.ErrAgentRunning):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lifecycle.ErrUpgradeNotSupported):
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
	svc.Cleanup()
	return firstErr
}

// bearerAuthMiddleware requires a valid Bearer token for /api/* routes.
// Health-check endpoints (/healthz, /readyz) are exempt.
func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			auth := r.Header.Get("Authorization")
			expected := "Bearer " + token
			// Use constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopback returns true when the host string resolves to a loopback address.
func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
