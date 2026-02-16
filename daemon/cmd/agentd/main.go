package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	"carrier/daemon/internal/api"
	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/config"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/logging"
	"carrier/daemon/internal/ratelimit"
)

const (
	shutdownTimeout = 30 * time.Second
	defaultLogsTail = 200
	maxLogsTail     = 1000
	// maxBodySize is the maximum allowed request body size (1 MB).
	maxBodySize = 1 << 20
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
	if err := svc.RegisterManifest(catalog.ZeroClawManifest()); err != nil {
		log.Fatalf("register zeroclaw manifest: %v", err)
	}
	if err := svc.RegisterManifest(catalog.PicoClawManifest()); err != nil {
		log.Fatalf("register picoclaw manifest: %v", err)
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

	// Initialize pairing code store and issue initial code
	pairStore := api.NewPairingCodeStore(nil)
	pairRecord, _ := pairStore.Issue(5 * time.Minute)
	fmt.Printf("\n  PAIR_CODE: %s\n  (expires in 5 minutes)\n\n", pairRecord.Code)

	// Build HTTP server
	ready := &atomic.Bool{}
	ready.Store(false)
	pairLimiter := ratelimit.New(ratelimit.WithMax(5), ratelimit.WithWindow(1*time.Minute))
	mux := buildHTTPMux(svc, ready, pairStore, pairLimiter)
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
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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

func buildHTTPMux(svc *lifecycle.Service, ready *atomic.Bool, pairStore *api.PairingCodeStore, pairLimiter *ratelimit.Limiter) *http.ServeMux {
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
		agentID, ok := extractAgentIDFromBody(w, r)
		if !ok {
			return
		}
		handleInstall(svc, agentID, w, r)
	})

	register("/api/start", func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := extractAgentIDFromBody(w, r)
		if !ok {
			return
		}
		handleStart(svc, agentID, w, r)
	})

	register("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := extractAgentIDFromBody(w, r)
		if !ok {
			return
		}
		handleStop(svc, agentID, w, r)
	})

	register("/api/status/", func(w http.ResponseWriter, r *http.Request) {
		raw := trimPathByPrefixes(r.URL.Path, "/api/status/", "/api/v1/status/")
		agentID, err := parsePathAgentID(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		handleStatus(svc, agentID, w, r)
	})

	register("/api/logs/", func(w http.ResponseWriter, r *http.Request) {
		raw := trimPathByPrefixes(r.URL.Path, "/api/logs/", "/api/v1/logs/")
		agentID, err := parsePathAgentID(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		handleLogs(svc, agentID, w, r)
	})

	register("/api/upgrade", func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := extractAgentIDFromBody(w, r)
		if !ok {
			return
		}
		handleUpgrade(svc, agentID, w, r)
	})

	register("/api/diagnose", func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := extractAgentIDFromBody(w, r)
		if !ok {
			return
		}
		handleDiagnose(svc, agentID, w, r)
	})

	// Pairing endpoints
	register("/api/pairing/codes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			codes := pairStore.List()
			writeJSON(w, http.StatusOK, map[string]interface{}{"codes": codes})
			return
		}
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	register("/api/pairing/verify-consume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ip := remoteIP(r)
		if pairLimiter != nil && !pairLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests, "too many pairing attempts, try again later")
			return
		}

		var body struct {
			Code string `json:"code"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := pairStore.VerifyAndConsume(body.Code); err != nil {
			if errors.Is(err, api.ErrPairCodeInvalid) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code":     body.Code,
			"consumed": true,
		})
	})

	// RESTful agent action routes: /api/v1/agents/{id}/{action}
	// Note: /api/v1/agents (exact match, list all) is already registered
	// via the register() helper above from /api/agents.
	mux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		// Handle special case: /api/v1/agents/status (all agents status)
		if r.URL.Path == "/api/v1/agents/status" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			agents := svc.ListAgents()
			writeJSON(w, http.StatusOK, map[string]interface{}{"statuses": agents})
			return
		}

		// Parse agent ID and action from path
		agentID, action, ok := parseAgentActionPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		// Delegate to the shared lifecycle handlers
		switch action {
		case "install":
			handleInstall(svc, agentID, w, r)
		case "start":
			handleStart(svc, agentID, w, r)
		case "stop":
			handleStop(svc, agentID, w, r)
		case "status":
			handleStatus(svc, agentID, w, r)
		case "logs":
			handleLogs(svc, agentID, w, r)
		case "upgrade":
			handleUpgrade(svc, agentID, w, r)
		case "diagnose":
			handleDiagnose(svc, agentID, w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// Diagnosis handoffs endpoint
	mux.HandleFunc("/api/v1/diagnosis/handoffs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			AgentID   string `json:"agentId"`
			Consent   bool   `json:"consent"`
			Actor     string `json:"actor"`
			RequestID string `json:"requestId"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.AgentID) == "" {
			writeJSONError(w, http.StatusBadRequest, "agentId is required")
			return
		}
		if err := validateAgentID(body.AgentID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		handoff, err := svc.CreateRemoteDiagnosisHandoff(body.AgentID, body.Consent, body.Actor, body.RequestID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":          handoff.ID,
			"agentId":     handoff.AgentID,
			"consent":     handoff.Consent,
			"artifactRef": handoff.ArtifactRef,
			"status":      handoff.Status,
			"createdAt":   handoff.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	})

	return mux
}

// extractAgentIDFromBody decodes an agent ID from a POST JSON body,
// validates it, and writes an error response on failure.
func extractAgentIDFromBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return "", false
	}
	var body agentIDBody
	if !decodeBody(w, r, &body) {
		return "", false
	}
	if err := validateAgentID(body.AgentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return body.AgentID, true
}

// Shared lifecycle handlers used by both legacy and v1 routes.

func handleInstall(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := svc.Install(r.Context(), agentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func handleStart(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := svc.Start(r.Context(), agentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func handleStop(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := svc.Stop(r.Context(), agentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func handleStatus(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	state, err := svc.Status(agentID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func handleLogs(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tail := parseLogsTail(r.URL.Query().Get("tail"))
	lines, err := svc.Logs(agentID, tail)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"lines": lines})
}

func handleUpgrade(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := svc.Upgrade(r.Context(), agentID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleDiagnose(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	artifactRef, err := svc.Diagnose(agentID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"artifactRef": artifactRef})
}

type agentIDBody struct {
	AgentID string `json:"agentId"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	// Cap request body to maxBodySize to prevent DoS.
	limited := io.LimitReader(r.Body, maxBodySize+1)
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	// Reject trailing JSON values (e.g. `{}{}`)
	if dec.More() {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: unexpected trailing data")
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

// parseAgentActionPath extracts agent ID and action from /api/v1/agents/{id}/{action}
func parseAgentActionPath(path string) (agentID string, action string, ok bool) {
	const prefix = "/api/v1/agents/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := path[len(prefix):]
	if strings.Contains(rest, "//") {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	rawAgentID := strings.TrimSpace(parts[0])
	action = strings.TrimSpace(parts[1])
	if rawAgentID == "" || action == "" {
		return "", "", false
	}
	decoded, err := url.PathUnescape(rawAgentID)
	if err != nil {
		return "", "", false
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" || strings.Contains(decoded, "/") || strings.Contains(decoded, "\\") || strings.Contains(decoded, "..") {
		return "", "", false
	}
	if !agentIDPattern.MatchString(decoded) {
		return "", "", false
	}
	return decoded, action, true
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

// remoteIP extracts the client IP from the request, preferring X-Forwarded-For.
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isLoopback returns true when the host string resolves to a loopback address.
func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
