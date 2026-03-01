// Package server exposes the daemon HTTP API as a callable Run function.
// This allows the unified carrier binary to launch daemon mode without
// duplicating bootstrap logic in cmd/carrier.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/logging"
	"carrier/daemon/internal/memory"
	"carrier/daemon/internal/messaging"
	"carrier/daemon/internal/ratelimit"
	"carrier/shared/config"
)

const (
	shutdownTimeout          = 30 * time.Second
	defaultReadHeaderTimeout = 10 * time.Second
	defaultReadTimeout       = 30 * time.Second
	// Long-running install/start requests can take several minutes on cold machines.
	// Keep write timeout above those command windows so clients do not get EOF mid-flight.
	defaultWriteTimeout = 30 * time.Minute
	defaultIdleTimeout  = 120 * time.Second
	defaultLogsTail     = 200
	maxLogsTail         = 1000
	maxBodySize         = 1 << 20
)

var agentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

var (
	userConfigDirFunc = os.UserConfigDir
	userHomeDirFunc   = os.UserHomeDir
	currentUserFunc   = user.Current
)

// Run starts the daemon HTTP API server. It blocks until a termination
// signal is received or the server encounters a fatal error.
func Run() {
	logger := logging.Init()
	logger.Info("initializing carrier daemon")

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

	memRoot, err := defaultMemoryRoot()
	if err != nil {
		log.Fatalf("resolve memory root: %v", err)
	}
	memStore := memory.NewStore(memory.WithRootDir(memRoot))
	opts = append(opts, lifecycle.WithMemoryStore(memStore))

	statePath, err := defaultLifecycleStatePath()
	if err != nil {
		log.Fatalf("resolve lifecycle state path: %v", err)
	}
	opts = append(opts, lifecycle.WithStateFile(statePath))
	alertsEnabled := parseEnabledEnv(os.Getenv("CARRIER_ALERTS_ENABLED"))
	alertWebhookURL := strings.TrimSpace(os.Getenv("CARRIER_ALERT_WEBHOOK_URL"))
	if alertsEnabled {
		opts = append(opts, lifecycle.WithAlertManager(lifecycle.NewAlertManager(true, lifecycle.WebhookAlertSink{
			URL: alertWebhookURL,
		})))
	}
	webhookURL := strings.TrimSpace(os.Getenv("CARRIER_WEBHOOK_URL"))
	webhookEvents := parseCSVEnv(os.Getenv("CARRIER_WEBHOOK_EVENTS"))
	opts = append(opts, lifecycle.WithWebhookManager(lifecycle.NewWebhookManager(webhookURL, webhookEvents)))

	svc := lifecycle.NewService(baseagent.NewLLMTriager(baseagent.NoopTriager{}), opts...)
	var baseMemoryStore baseagent.MemoryStore
	if memStore != nil {
		baseMemoryStore = newBaseAgentMemoryStoreAdapter(memStore)
	}
	baseRuntime := baseagent.NewRuntime(newLifecycleAgentServiceAdapter(svc), baseMemoryStore)

	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		log.Fatalf("register openclaw manifest: %v", err)
	}
	if err := svc.RegisterManifest(catalog.ZeroClawManifest()); err != nil {
		log.Printf("WARN: register zeroclaw manifest: %v (skipping)", err)
	}
	if err := svc.RegisterManifest(catalog.PicoClawManifest()); err != nil {
		log.Printf("WARN: register picoclaw manifest: %v (skipping)", err)
	}
	if err := svc.RegisterManifest(catalog.CodexManifest()); err != nil {
		log.Printf("WARN: register codex manifest: %v (skipping)", err)
	}
	if err := svc.RegisterManifest(catalog.OpenCodeManifest()); err != nil {
		log.Printf("WARN: register opencode manifest: %v (skipping)", err)
	}

	logger.Info("carrier daemon booted")
	fmt.Printf("carrier daemon booted (listen=%s:%d log=%s/%s)\n",
		cfg.Server.Host, cfg.Server.Port, cfg.Log.Level, cfg.Log.Format)
	fmt.Println("catalog (active):")
	for _, entry := range catalog.ActiveEntries() {
		fmt.Printf("- %s (%s): %s\n", entry.Name, entry.ID, entry.Status)
	}
	fmt.Println("catalog (candidate, unlisted):")
	for _, entry := range catalog.ListByStatus(catalog.StatusCandidate) {
		fmt.Printf("- %s (%s): %s [candidate]\n", entry.Name, entry.ID, entry.Status)
	}

	if cfg.Server.APIToken == "" && !isLoopback(cfg.Server.Host) {
		log.Fatalf("CARRIER_SERVER_API_TOKEN must be set when listening on non-loopback address %q", cfg.Server.Host)
	}
	if cfg.Server.APIToken == "" {
		cfg.Server.Host = "127.0.0.1"
		logger.Info("no API token configured; forcing loopback-only bind (127.0.0.1)")
	}

	pairStore := api.NewPairingCodeStore(nil)
	pairRecord, _ := pairStore.Issue(5 * time.Minute)
	fmt.Printf("\n  PAIR_CODE: %s\n  (expires in 5 minutes)\n\n", pairRecord.Code)

	ready := &atomic.Bool{}
	ready.Store(false)
	pairLimiter := ratelimit.New(ratelimit.WithMax(5), ratelimit.WithWindow(1*time.Minute))
	msgBus := messaging.NewMessageBus()
	mux := buildHTTPMuxWithBaseAgent(svc, baseRuntime, ready, pairStore, pairLimiter, msgBus)
	var handler http.Handler = mux
	if cfg.Server.APIToken != "" {
		handler = bearerAuthMiddleware(cfg.Server.APIToken, mux)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := newHTTPServer(addr, handler)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	serverErrCh := make(chan error, 1)
	go func() {
		fmt.Printf("HTTP API listening on %s\n", addr)
		serverErrCh <- httpServer.Serve(ln)
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "http server shutdown error: %v\n", err)
	}

	if err := shutdownAgents(svc, shutdownTimeout); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	fmt.Println("carrier daemon stopped gracefully")
}

func buildHTTPMux(
	svc *lifecycle.Service,
	ready *atomic.Bool,
	pairStore *api.PairingCodeStore,
	pairLimiter *ratelimit.Limiter,
) *http.ServeMux {
	return buildHTTPMuxWithBaseAgent(svc, nil, ready, pairStore, pairLimiter)
}

func buildHTTPMuxWithBaseAgent(
	svc *lifecycle.Service,
	baseRuntime *baseagent.Runtime,
	ready *atomic.Bool,
	pairStore *api.PairingCodeStore,
	pairLimiter *ratelimit.Limiter,
	messageBuses ...*messaging.MessageBus,
) *http.ServeMux {
	mux := http.NewServeMux()
	var msgBus *messaging.MessageBus
	if len(messageBuses) > 0 {
		msgBus = messageBuses[0]
	}
	if msgBus == nil {
		msgBus = messaging.NewMessageBus()
	}
	memStore := svc.MemoryStore()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
		if strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/v2/") {
			mux.HandleFunc("/api/v1"+strings.TrimPrefix(path, "/api"), handler)
		}
	}

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

	register("/api/base-agent/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if baseRuntime == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "base agent runtime is unavailable")
			return
		}
		var body baseagent.ChatRequest
		if !decodeBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Message) == "" {
			writeJSONError(w, http.StatusBadRequest, "message is required")
			return
		}
		resp, err := baseRuntime.Chat(r.Context(), body)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	register("/api/v2/memory/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject             string   `json:"subject"`
			Query               string   `json:"query"`
			MaxResults          int      `json:"maxResults"`
			MinScore            float64  `json:"minScore"`
			CandidateMultiplier int      `json:"candidateMultiplier"`
			AdaptiveRecall      *bool    `json:"adaptiveRecall"`
			Rerank              *bool    `json:"rerank"`
			LexicalWeight       *float64 `json:"lexicalWeight"`
			SemanticWeight      *float64 `json:"semanticWeight"`
			IncludeDistilled    *bool    `json:"includeDistilled"`
			IncludeRaw          *bool    `json:"includeRaw"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		results := memStore.Search(memory.SearchOptions{
			Subject:             body.Subject,
			Query:               body.Query,
			MaxResults:          body.MaxResults,
			MinScore:            body.MinScore,
			CandidateMultiplier: body.CandidateMultiplier,
			AdaptiveRecall:      body.AdaptiveRecall,
			Rerank:              body.Rerank,
			LexicalWeight:       body.LexicalWeight,
			SemanticWeight:      body.SemanticWeight,
			IncludeDistilled:    body.IncludeDistilled,
			IncludeRaw:          body.IncludeRaw,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
	})

	register("/api/v2/memory/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject string `json:"subject"`
			ID      string `json:"id"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		record, err := memStore.GetRecord(body.Subject, body.ID)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"record": record})
	})

	register("/api/v2/memory/observe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject       string   `json:"subject"`
			AgentID       string   `json:"agentId"`
			AppID         string   `json:"appId"`
			SessionID     string   `json:"sessionId"`
			Scope         string   `json:"scope"`
			ToolName      string   `json:"toolName"`
			InputsDigest  string   `json:"inputsDigest"`
			OutputSnippet string   `json:"outputSnippet"`
			Status        string   `json:"status"`
			Artifacts     []string `json:"artifacts"`
			Labels        []string `json:"labels"`
			AutoCurate    bool     `json:"autoCurate"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		ev, err := memStore.Observe(memory.ObserveInput{
			Subject:       body.Subject,
			AgentID:       body.AgentID,
			AppID:         body.AppID,
			SessionID:     body.SessionID,
			Scope:         memory.Scope(body.Scope),
			ToolName:      body.ToolName,
			InputsDigest:  body.InputsDigest,
			OutputSnippet: body.OutputSnippet,
			Status:        body.Status,
			Artifacts:     body.Artifacts,
			Labels:        body.Labels,
			AutoCurate:    body.AutoCurate,
		})
		if err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"event": ev})
	})

	register("/api/v2/memory/records/upsert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject        string   `json:"subject"`
			ID             string   `json:"id"`
			Scope          string   `json:"scope"`
			Type           string   `json:"type"`
			ContentRaw     string   `json:"contentRaw"`
			ContentSummary string   `json:"contentSummary"`
			Tags           []string `json:"tags"`
			Provenance     string   `json:"provenance"`
			Confidence     float64  `json:"confidence"`
			Importance     int      `json:"importance"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		rec, err := memStore.UpsertRecord(memory.UpsertRecordInput{
			Subject:        body.Subject,
			ID:             body.ID,
			Scope:          memory.Scope(body.Scope),
			Type:           memory.RecordType(body.Type),
			ContentRaw:     body.ContentRaw,
			ContentSummary: body.ContentSummary,
			Tags:           body.Tags,
			Provenance:     body.Provenance,
			Confidence:     body.Confidence,
			Importance:     body.Importance,
		})
		if err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"record": rec})
	})

	register("/api/v2/memory/records/archive", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject string `json:"subject"`
			ID      string `json:"id"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := memStore.ArchiveRecord(body.Subject, body.ID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "archived"})
	})

	register("/api/v2/memory/grants/grant", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Subject   string `json:"subject"`
			Scope     string `json:"scope"`
			GrantedBy string `json:"grantedBy"`
			Reason    string `json:"reason"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		grant, err := memStore.GrantScope(body.Subject, memory.Scope(body.Scope), body.GrantedBy, body.Reason)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"grant": grant})
	})

	register("/api/v2/memory/grants/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			GrantID   string `json:"grantId"`
			RevokedBy string `json:"revokedBy"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := memStore.RevokeScope(body.GrantID, body.RevokedBy); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "revoked"})
	})

	register("/api/v2/memory/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"audit": memStore.AuditLogs()})
	})

	register("/api/v2/memory/instance/attach", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			InstanceID string `json:"instanceId"`
			Scope      string `json:"scope"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := memStore.AttachScope(body.InstanceID, memory.Scope(body.Scope)); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "attached"})
	})

	register("/api/v2/memory/instance/detach", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			InstanceID string `json:"instanceId"`
			Scope      string `json:"scope"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := memStore.DetachScope(body.InstanceID, memory.Scope(body.Scope)); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "detached"})
	})

	register("/api/v2/memory/instance/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			InstanceID  string `json:"instanceId"`
			Path        string `json:"path"`
			TargetScope string `json:"targetScope"`
			Actor       string `json:"actor"`
			RequestID   string `json:"requestId"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		id, err := memStore.ImportForInstance(body.InstanceID, body.Path, memory.InstanceImportOptions{
			Actor:       body.Actor,
			RequestID:   body.RequestID,
			TargetScope: memory.Scope(body.TargetScope),
		})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": id})
	})

	register("/api/v2/memory/instance/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			InstanceID string `json:"instanceId"`
			Format     string `json:"format"`
			Actor      string `json:"actor"`
			RequestID  string `json:"requestId"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		ref, err := memStore.ExportForInstance(body.InstanceID, memory.InstanceExportOptions{
			Actor:     body.Actor,
			RequestID: body.RequestID,
			Format:    body.Format,
		})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"artifactRef": ref})
	})

	register("/api/v2/memory/instance/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			InstanceID                 string                      `json:"instanceId"`
			Scope                      string                      `json:"scope"`
			DryRun                     bool                        `json:"dryRun"`
			Force                      bool                        `json:"force"`
			Actor                      string                      `json:"actor"`
			RequestID                  string                      `json:"requestId"`
			Reason                     string                      `json:"reason"`
			BackendHint                string                      `json:"backendHint"`
			MinSourceAgeDays           int                         `json:"minSourceAgeDays"`
			MaxSourceRecords           int                         `json:"maxSourceRecords"`
			MaxSummaryTokens           int                         `json:"maxSummaryTokens"`
			ClusterSimilarityThreshold float64                     `json:"clusterSimilarityThreshold"`
			SkipRecentHours            int                         `json:"skipRecentHours"`
			DistillScoreThreshold      float64                     `json:"distillScoreThreshold"`
			ScoreWeights               *memory.DistillScoreWeights `json:"scoreWeights"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		result, err := memStore.DistillForInstance(r.Context(), memory.InstanceDistillOptions{
			Actor:                       body.Actor,
			RequestID:                   body.RequestID,
			InstanceID:                  body.InstanceID,
			Scope:                       memory.Scope(body.Scope),
			DryRun:                      body.DryRun,
			Force:                       body.Force,
			Reason:                      body.Reason,
			BackendHint:                 body.BackendHint,
			MinSourceAgeDays:            body.MinSourceAgeDays,
			MaxSourceRecords:            body.MaxSourceRecords,
			MaxSummaryTokens:            body.MaxSummaryTokens,
			ClusterSimilarityThreshold:  body.ClusterSimilarityThreshold,
			SkipRecentHours:             body.SkipRecentHours,
			DistillScoreThreshold:       body.DistillScoreThreshold,
			DistillScoreWeightsOverride: body.ScoreWeights,
		})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
	})

	register("/api/v2/memory/migrate/backup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			Actor     string `json:"actor"`
			RequestID string `json:"requestId"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		path, err := memStore.CreateMigrationBackup(body.Actor, body.RequestID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"backupPath": path})
	})

	register("/api/v2/memory/migrate/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"validation": memStore.ValidateMigration()})
	})

	register("/api/v2/memory/migrate/rollback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if memStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "memory store is unavailable")
			return
		}
		var body struct {
			BackupPath string `json:"backupPath"`
			Actor      string `json:"actor"`
			RequestID  string `json:"requestId"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := memStore.RollbackFromBackup(body.BackupPath, body.Actor, body.RequestID); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "rolled_back"})
	})

	mux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agents/status" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			agents := svc.ListAgents()
			writeJSON(w, http.StatusOK, map[string]interface{}{"statuses": agents})
			return
		}
		if agentID, action, ok := parseAgentMessagingPath(r.URL.Path); ok {
			switch action {
			case "send":
				handleMessageSend(msgBus, agentID, w, r)
			case "inbox":
				handleMessageInbox(msgBus, agentID, w, r)
			default:
				http.NotFound(w, r)
			}
			return
		}

		agentID, action, ok := parseAgentActionPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

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
		case "metrics":
			handleMetrics(svc, agentID, w, r)
		case "config":
			handleConfigSet(svc, agentID, w, r)
		case "upgrade":
			handleUpgrade(svc, agentID, w, r)
		case "uninstall":
			handleUninstall(svc, agentID, w, r)
		case "diagnose":
			handleDiagnose(svc, agentID, w, r)
		default:
			http.NotFound(w, r)
		}
	})

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

	mux.Handle("/", webUIHandler())

	return mux
}

func handleMessageSend(bus *messaging.MessageBus, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body messaging.Message
	if !decodeBody(w, r, &body) {
		return
	}
	body.To = agentID
	if err := bus.Send(body); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func handleMessageInbox(bus *messaging.MessageBus, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	msg, err := bus.Receive(agentID, 100*time.Millisecond)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"messages": []messaging.Message{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": []messaging.Message{msg}})
}

type lifecycleAgentServiceAdapter struct {
	svc *lifecycle.Service
}

type baseAgentMemoryStoreAdapter struct {
	store *memory.Store
}

func newBaseAgentMemoryStoreAdapter(store *memory.Store) *baseAgentMemoryStoreAdapter {
	if store == nil {
		return nil
	}
	return &baseAgentMemoryStoreAdapter{store: store}
}

func (a *baseAgentMemoryStoreAdapter) Get(id string) error {
	_, err := a.store.Get(id)
	return err
}

func (a *baseAgentMemoryStoreAdapter) Create(id, name, version string, memType baseagent.MemoryType, owner string) error {
	_, err := a.store.Create(id, name, version, memory.Type(memType), owner)
	return err
}

func (a *baseAgentMemoryStoreAdapter) List() []baseagent.MemoryEntry {
	raw := a.store.List()
	out := make([]baseagent.MemoryEntry, 0, len(raw))
	for _, item := range raw {
		out = append(out, baseagent.MemoryEntry{
			ID:    item.ID,
			State: baseagent.MemoryState(item.State),
		})
	}
	return out
}

func (a *baseAgentMemoryStoreAdapter) SetAttachmentsFromLinks(agentID string, memoryIDs []string) error {
	return a.store.SetAttachmentsFromLinks(agentID, memoryIDs)
}

func (a *baseAgentMemoryStoreAdapter) PrepareAgentMemory(agentID string) error {
	_, err := a.store.PrepareAgentMemory(agentID)
	return err
}

func (a *baseAgentMemoryStoreAdapter) ExportMemory(memoryID string, opts baseagent.ExportOptions) (string, error) {
	return a.store.ExportMemory(memoryID, memory.ExportOptions{
		Actor:     opts.Actor,
		RequestID: opts.RequestID,
	})
}

func (a *baseAgentMemoryStoreAdapter) Archive(memoryID string) error {
	return a.store.Archive(memoryID)
}

func (a *baseAgentMemoryStoreAdapter) Search(subject, query string, maxResults int, minScore float64) ([]baseagent.MemorySearchHit, error) {
	hits := a.store.Search(memory.SearchOptions{
		Subject:    subject,
		Query:      query,
		MaxResults: maxResults,
		MinScore:   minScore,
	})
	out := make([]baseagent.MemorySearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, baseagent.MemorySearchHit{
			ID:         h.ID,
			Scope:      string(h.Scope),
			Score:      h.Score,
			Snippet:    h.Snippet,
			Provenance: h.Provenance,
		})
	}
	return out, nil
}

func (a *baseAgentMemoryStoreAdapter) GetRecord(subject, id string) (baseagent.MemoryRecord, error) {
	rec, err := a.store.GetRecord(subject, id)
	if err != nil {
		return baseagent.MemoryRecord{}, err
	}
	return baseagent.MemoryRecord{
		ID:             rec.ID,
		Scope:          string(rec.Scope),
		Type:           string(rec.Type),
		ContentRaw:     rec.ContentRaw,
		ContentSummary: rec.ContentSummary,
		Provenance:     rec.Provenance,
	}, nil
}

func (a *baseAgentMemoryStoreAdapter) Observe(subject, toolName, outputSnippet, scope string) (string, error) {
	ev, err := a.store.Observe(memory.ObserveInput{
		Subject:       subject,
		AgentID:       subject,
		Scope:         memory.Scope(scope),
		ToolName:      toolName,
		OutputSnippet: outputSnippet,
		AutoCurate:    true,
	})
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}

func (a *baseAgentMemoryStoreAdapter) Grant(subject, scope, grantedBy, reason string) (string, error) {
	grant, err := a.store.GrantScope(subject, memory.Scope(scope), grantedBy, reason)
	if err != nil {
		return "", err
	}
	return grant.ID, nil
}

func (a *baseAgentMemoryStoreAdapter) Revoke(grantID, revokedBy string) error {
	return a.store.RevokeScope(grantID, revokedBy)
}

func (a *baseAgentMemoryStoreAdapter) ListAudits() []baseagent.MemoryAudit {
	audits := a.store.AuditLogs()
	out := make([]baseagent.MemoryAudit, 0, len(audits))
	for _, item := range audits {
		out = append(out, baseagent.MemoryAudit{
			Action:    item.Action,
			Target:    item.Target,
			Result:    item.Result,
			Message:   item.Message,
			Timestamp: item.Timestamp.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

func newLifecycleAgentServiceAdapter(svc *lifecycle.Service) *lifecycleAgentServiceAdapter {
	return &lifecycleAgentServiceAdapter{svc: svc}
}

func (a *lifecycleAgentServiceAdapter) ListAgents() []baseagent.AgentState {
	states := a.svc.ListAgents()
	out := make([]baseagent.AgentState, 0, len(states))
	for _, s := range states {
		out = append(out, toBaseAgentState(s))
	}
	return out
}

func (a *lifecycleAgentServiceAdapter) Install(ctx context.Context, agentID string) error {
	return a.svc.Install(ctx, agentID)
}

func (a *lifecycleAgentServiceAdapter) Uninstall(ctx context.Context, agentID string) error {
	return a.svc.Uninstall(ctx, agentID)
}

func (a *lifecycleAgentServiceAdapter) Start(ctx context.Context, agentID string) error {
	return a.svc.Start(ctx, agentID)
}

func (a *lifecycleAgentServiceAdapter) Stop(ctx context.Context, agentID string) error {
	return a.svc.Stop(ctx, agentID)
}

func (a *lifecycleAgentServiceAdapter) Status(agentID string) (baseagent.AgentState, error) {
	state, err := a.svc.Status(agentID)
	if err != nil {
		return baseagent.AgentState{}, err
	}
	return toBaseAgentState(state), nil
}

func (a *lifecycleAgentServiceAdapter) Logs(agentID string, tail int) ([]string, error) {
	return a.svc.Logs(agentID, tail)
}

func (a *lifecycleAgentServiceAdapter) Upgrade(ctx context.Context, agentID string) (baseagent.UpgradeResult, error) {
	result, err := a.svc.Upgrade(ctx, agentID)
	if err != nil {
		return baseagent.UpgradeResult{}, err
	}
	return baseagent.UpgradeResult{
		AgentID:     result.AgentID,
		FromVersion: result.FromVersion,
		ToVersion:   result.ToVersion,
	}, nil
}

func (a *lifecycleAgentServiceAdapter) Diagnose(agentID string) (string, error) {
	return a.svc.Diagnose(agentID)
}

func toBaseAgentState(s lifecycle.AgentState) baseagent.AgentState {
	return baseagent.AgentState{
		ID:           s.ID,
		Install:      string(s.Install),
		Runtime:      string(s.Runtime),
		Health:       string(s.Health),
		RestartCount: s.RestartCount,
	}
}

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

func handleInstall(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var instanceName string
	var wantsMultiInstance bool
	if r.Body != nil && r.ContentLength != 0 {
		var body struct {
			AgentID       string `json:"agentId"`
			InstanceName  string `json:"instance_name"`
			MultiInstance bool   `json:"multi_instance"`
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &body)
			instanceName = body.InstanceName
			wantsMultiInstance = body.MultiInstance
		}
	}

	if instanceName != "" || wantsMultiInstance {
		instID, err := svc.RegisterInstance(agentID, instanceName)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.Install(r.Context(), instID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "installed", "instance_id": instID})
		return
	}

	if err := svc.Install(r.Context(), agentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
}

func handleUninstall(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := svc.Uninstall(r.Context(), agentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}

func handleStart(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	opts := lifecycle.StartOptions{}
	if strings.HasPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/agents/") {
		parsed, parseErr := parseStartOptionsFromRequest(r)
		if parseErr != nil {
			writeJSONError(w, http.StatusBadRequest, parseErr.Error())
			return
		}
		opts = parsed
	}
	if err := svc.StartWithOptions(r.Context(), agentID, opts); err != nil {
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

func handleMetrics(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	metrics, err := svc.Metrics(agentID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func handleConfigSet(svc *lifecycle.Service, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Changes map[string]string `json:"changes"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := svc.HotReloadConfig(agentID, body.Changes); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
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
	if r.ContentLength > maxBodySize {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("request body too large: max %d bytes", maxBodySize))
		return false
	}

	limited := io.LimitReader(r.Body, maxBodySize+1)
	defer r.Body.Close()
	raw, err := io.ReadAll(limited)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	if len(raw) > maxBodySize {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("request body too large: max %d bytes", maxBodySize))
		return false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: empty body")
		return false
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	var trailing struct{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: unexpected trailing data")
		return false
	}
	return true
}

func parseStartOptionsFromRequest(r *http.Request) (lifecycle.StartOptions, error) {
	if r == nil || r.Body == nil {
		return lifecycle.StartOptions{}, nil
	}
	limited := io.LimitReader(r.Body, maxBodySize+1)
	defer r.Body.Close()
	raw, err := io.ReadAll(limited)
	if err != nil {
		return lifecycle.StartOptions{}, fmt.Errorf("invalid request body: %w", err)
	}
	if len(raw) > maxBodySize {
		return lifecycle.StartOptions{}, fmt.Errorf("request body too large: max %d bytes", maxBodySize)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return lifecycle.StartOptions{}, nil
	}
	var payload struct {
		Isolation bool `json:"isolation,omitempty"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return lifecycle.StartOptions{}, fmt.Errorf("invalid request body: %w", err)
	}
	return lifecycle.StartOptions{Isolation: payload.Isolation}, nil
}

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

func parseAgentMessagingPath(path string) (agentID string, action string, ok bool) {
	const prefix = "/api/v1/agents/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return "", "", false
	}
	if parts[1] != "messages" {
		return "", "", false
	}
	decoded, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", "", false
	}
	if err := validateAgentID(decoded); err != nil {
		return "", "", false
	}
	act := strings.TrimSpace(parts[2])
	if act != "send" && act != "inbox" {
		return "", "", false
	}
	return decoded, act, true
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

func parseEnabledEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseCSVEnv(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
	case errors.Is(err, lifecycle.ErrIsolationUnavailable):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, lifecycle.ErrIsolationStartFailed):
		writeJSONError(w, http.StatusBadGateway, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}
}

func shutdownAgents(svc *lifecycle.Service, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- stopAllAgents(ctx, svc)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "shutdown timed out after", timeout)
		return ctx.Err()
	}
}

func stopAllAgents(ctx context.Context, svc *lifecycle.Service) error {
	agents := svc.ListAgents()

	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)

	for _, agent := range agents {
		if agent.Runtime == lifecycle.RuntimeStateRunning {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				if err := svc.Stop(ctx, id); err != nil {
					fmt.Fprintf(os.Stderr, "failed to stop agent %s: %v\n", id, err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}(agent.ID)
		}
	}

	wg.Wait()
	svc.Cleanup()
	return firstErr
}

func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth required for /api/ endpoints and /healthz (when token is set).
		needsAuth := strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz"
		if needsAuth {
			// Accept either "Authorization: Bearer <token>" or "X-Gateway-Token: <token>".
			auth := r.Header.Get("Authorization")
			if auth == "" {
				if gt := r.Header.Get("X-Gateway-Token"); gt != "" {
					auth = "Bearer " + gt
				}
			}
			expected := "Bearer " + token
			authDigest := sha256.Sum256([]byte(auth))
			expectedDigest := sha256.Sum256([]byte(expected))
			if subtle.ConstantTimeCompare(authDigest[:], expectedDigest[:]) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

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

func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultMemoryRoot() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("CARRIER_MEMORY_ROOT")); raw != "" {
		return raw, nil
	}
	if configDir, err := userConfigDirFunc(); err == nil {
		if trimmed := strings.TrimSpace(configDir); trimmed != "" {
			return filepath.Join(trimmed, "carrier", "memory"), nil
		}
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "carrier", "memory"), nil
}

func defaultLifecycleStatePath() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("CARRIER_LIFECYCLE_STATE_FILE")); raw != "" {
		return raw, nil
	}
	if configDir, err := userConfigDirFunc(); err == nil {
		if trimmed := strings.TrimSpace(configDir); trimmed != "" {
			return filepath.Join(trimmed, "carrier", "lifecycle-state.json"), nil
		}
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "carrier", "lifecycle-state.json"), nil
}

func resolveDaemonHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}
	if home, err := userHomeDirFunc(); err == nil {
		if trimmed := strings.TrimSpace(home); trimmed != "" {
			return trimmed, nil
		}
	}
	if current, err := currentUserFunc(); err == nil && current != nil {
		if trimmed := strings.TrimSpace(current.HomeDir); trimmed != "" {
			return trimmed, nil
		}
	}

	if runtime.GOOS == "windows" {
		if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
			return profile, nil
		}
		if drive, homePath := strings.TrimSpace(os.Getenv("HOMEDRIVE")), strings.TrimSpace(os.Getenv("HOMEPATH")); drive != "" && homePath != "" {
			return filepath.Clean(drive + homePath), nil
		}
	} else {
		switch username := strings.TrimSpace(os.Getenv("USER")); username {
		case "root":
			return "/root", nil
		case "":
			return "/root", nil
		default:
			return filepath.Join("/home", username), nil
		}
	}

	return "", errors.New("home directory unavailable")
}
