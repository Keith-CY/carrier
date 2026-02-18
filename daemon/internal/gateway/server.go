package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxCommandBodyBytes = 64 * 1024 // 64 KB
	discordMaxAgeSec           = 300
	gatewayReadHeaderTimeout   = 10 * time.Second
	gatewayReadTimeout         = 60 * time.Second
	gatewayWriteTimeout        = 90 * time.Second
	gatewayIdleTimeout         = 120 * time.Second
)

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

// buildGatewayMux constructs the gateway HTTP mux.
func buildGatewayMux(cfg *GatewayConfig, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore, setup *SetupStore) http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Command endpoint
	mux.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		requestID := requestIDFromCtx(r.Context())

		if err := checkGatewayToken(r, cfg.APIToken); err != nil {
			writeJSON(w, http.StatusUnauthorized, gatewayErrBody(err.code, err.msg))
			return
		}

		input, sessionToken, status, cmdErr := parseCommandRequest(r, cfg.MaxCommandBodyBytes)
		if cmdErr != nil {
			writeJSON(w, status, gatewayErrBody(cmdErr.code, cmdErr.msg))
			return
		}
		if input == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must provide command input"))
			return
		}

		// Auth validation before dispatching (for non-/pair commands)
		if authErr := validateCommandAuth(input, sessionToken, sessions); authErr != nil {
			writeJSON(w, http.StatusUnauthorized, authErr)
			return
		}

		input = InjectSessionToken(input, sessionToken)
		resp := SafeHandleCommand(r.Context(), input, daemon, sessions, downloads, rl, onboard)
		_ = requestID
		writeJSON(w, http.StatusOK, resp)
	})

	// Provider setup
	mux.HandleFunc("/api/v1/setup", func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromCtx(r.Context())
		if err := checkGatewayToken(r, cfg.APIToken); err != nil {
			writeJSON(w, http.StatusUnauthorized, gatewayErrBody(err.code, err.msg))
			return
		}
		switch r.Method {
		case http.MethodPost:
			handleSetupPost(w, r, requestID, setup)
		case http.MethodGet:
			handleSetupGet(w, r, requestID, setup)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		}
	})
	// Legacy alias
	mux.HandleFunc("/setup", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1/setup"
		mux.ServeHTTP(w, r)
	})

	// Webhooks
	mux.HandleFunc("/webhook/telegram", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		handleTelegramWebhook(w, r, cfg, daemon, sessions, downloads, rl, onboard)
	})
	mux.HandleFunc("/webhook/discord", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		handleDiscordWebhook(w, r, cfg, daemon, sessions, downloads, rl, onboard)
	})
	mux.HandleFunc("/webhook/feishu", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		handleFeishuWebhook(w, r, cfg, daemon, sessions, downloads, rl, onboard)
	})

	// Downloads
	mux.HandleFunc("/downloads/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		handleDownload(w, r, cfg, downloads)
	})

	// SSE logs streaming endpoint (polling fallback)
	mux.HandleFunc("/api/v1/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if err := checkGatewayToken(r, cfg.APIToken); err != nil {
			writeJSON(w, http.StatusUnauthorized, gatewayErrBody(err.code, err.msg))
			return
		}
		agentID := r.URL.Query().Get("agent")
		if agentID == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "agent query parameter required"))
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "streaming not supported"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := r.Context()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				resp := SafeHandleCommand(ctx, "logs "+agentID, daemon, sessions, downloads, rl, onboard)
				text := resp.Message
				if text != "" {
					for _, line := range strings.Split(text, "\n") {
						fmt.Fprintf(w, "data: %s\n", line)
					}
					fmt.Fprint(w, "\n")
					flusher.Flush()
				}
			}
		}
	})

	// Serve WebUI static files at root (catch-all, after API routes).
	// The handler is provided by webui_embed.go (with -tags webui) or
	// webui_stub.go (returns 404 when built without the tag).
	mux.Handle("/", webUIHandler())

	// Wrap with request-ID middleware
	return requestIDMiddleware(mux)
}

// requestIDMiddleware adds X-Request-Id to every request/response.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		incoming := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		// Strip control characters
		var cleaned strings.Builder
		for _, c := range incoming {
			if c >= 0x20 && c != 0x7F {
				cleaned.WriteRune(c)
			}
		}
		requestID := cleaned.String()
		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromCtx(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// --- Webhook handlers ---

func handleTelegramWebhook(w http.ResponseWriter, r *http.Request, cfg *GatewayConfig, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore) {
	requestID := requestIDFromCtx(r.Context())
	body, err := readBodyWithLimit(r, cfg.MaxCommandBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, gatewayErrBody("E_PAYLOAD_TOO_LARGE", err.Error()))
		return
	}

	if !VerifyTelegramSecret(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), cfg.TelegramWebhookSecret) {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_TELEGRAM_VERIFICATION_FAILED", "telegram webhook secret verification failed"))
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}

	nc := ParseTelegramUpdate(payload)
	if nc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "message": "ignored non-command telegram update"})
		return
	}

	session := sessions.GetSession("telegram", nc.ChatID)
	var sessionToken string
	if session != nil {
		sessionToken = session.SessionToken
	}
	input := InjectSessionToken(ToGatewayInput(nc), sessionToken)
	resp := SafeHandleCommand(r.Context(), input, daemon, sessions, downloads, rl, onboard)
	writeJSON(w, http.StatusOK, RenderTelegramResponse(resp))
}

func handleDiscordWebhook(w http.ResponseWriter, r *http.Request, cfg *GatewayConfig, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore) {
	requestID := requestIDFromCtx(r.Context())
	body, err := readBodyWithLimit(r, cfg.MaxCommandBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, gatewayErrBody("E_PAYLOAD_TOO_LARGE", err.Error()))
		return
	}

	if !VerifyDiscordSignature(r, body, cfg.DiscordPublicKey, discordMaxAgeSec) {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_DISCORD_SIGNATURE_INVALID", "discord request signature verification failed"))
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}

	// Ping (type=1)
	if t := toFloat(payload["type"]); t == 1 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"type": 1})
		return
	}

	nc := ParseDiscordPayload(payload)
	if nc == nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "unsupported discord payload"))
		return
	}

	session := sessions.GetSession("discord", nc.ChatID)
	var sessionToken string
	if session != nil {
		sessionToken = session.SessionToken
	}
	input := InjectSessionToken(ToGatewayInput(nc), sessionToken)
	resp := SafeHandleCommand(r.Context(), input, daemon, sessions, downloads, rl, onboard)
	rendered := RenderDiscordResponse(resp)

	_ = requestID
	// Slash command interaction (type=2) → wrap in interaction response
	if t := toFloat(payload["type"]); t == 2 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"type": 4,
			"data": rendered,
		})
		return
	}
	writeJSON(w, http.StatusOK, rendered)
}

func handleFeishuWebhook(w http.ResponseWriter, r *http.Request, cfg *GatewayConfig, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore) {
	requestID := requestIDFromCtx(r.Context())
	body, err := readBodyWithLimit(r, cfg.MaxCommandBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, gatewayErrBody("E_PAYLOAD_TOO_LARGE", err.Error()))
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}

	if !VerifyFeishuToken(payload, cfg.FeishuVerificationToken) {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_FEISHU_VERIFICATION_FAILED", "feishu event token verification failed"))
		return
	}

	// URL verification challenge
	if challenge, ok := ExtractFeishuChallenge(payload); ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{"challenge": challenge})
		return
	}

	nc := ParseFeishuEvent(payload)
	if nc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "message": "ignored non-command feishu event"})
		return
	}

	session := sessions.GetSession("feishu", nc.ChatID)
	var sessionToken string
	if session != nil {
		sessionToken = session.SessionToken
	}
	input := InjectSessionToken(ToGatewayInput(nc), sessionToken)
	resp := SafeHandleCommand(r.Context(), input, daemon, sessions, downloads, rl, onboard)
	writeJSON(w, http.StatusOK, RenderFeishuResponse(resp))
}

func handleDownload(w http.ResponseWriter, r *http.Request, cfg *GatewayConfig, downloads *DownloadStore) {
	requestID := requestIDFromCtx(r.Context())

	token, requestedFileName, ok := ParseDownloadPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"requestId": requestID,
			"result":    "error",
			"errorCode": "E_USAGE",
			"message":   "invalid download path",
		})
		return
	}

	tok := downloads.Consume(token)
	if tok == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"requestId": requestID,
			"result":    "error",
			"errorCode": "E_DOWNLOAD_TOKEN_INVALID",
			"message":   "download token is invalid or expired",
		})
		return
	}

	expectedName := ExpectedFileName(tok.FileRef)
	if requestedFileName != expectedName {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"requestId": requestID,
			"result":    "error",
			"errorCode": "E_DOWNLOAD_FILE_MISMATCH",
			"message":   "requested filename does not match token artifact",
		})
		return
	}

	// Security: validate artifact root
	if cfg.ArtifactRoot != "" {
		resolved, err := filepath.Abs(tok.FileRef)
		if err != nil || !IsPathUnderRoot(resolved, cfg.ArtifactRoot) {
			log.Printf("[gateway/downloads] path outside artifact root: %s (root: %s)", tok.FileRef, cfg.ArtifactRoot)
			writeJSON(w, http.StatusNotFound, map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"errorCode": "E_DOWNLOAD_NOT_FOUND",
				"message":   "artifact file was not found",
			})
			return
		}
	}

	data, err := os.ReadFile(tok.FileRef)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"requestId": requestID,
			"result":    "error",
			"errorCode": "E_DOWNLOAD_NOT_FOUND",
			"message":   "artifact file was not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", BuildContentDisposition(expectedName))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	if tok.SingleUse {
		downloads.FinalizeConsumed(tok.Token)
	}
}

func handleSetupPost(w http.ResponseWriter, r *http.Request, requestID string, setup *SetupStore) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "failed to read request body"))
		return
	}
	var req struct {
		Provider      string `json:"provider"`
		Token         string `json:"token"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"requestId": requestID, "result": "error",
			"errorCode": "E_MISSING_PROVIDER", "message": "provider field is required",
		})
		return
	}
	if !IsValidProviderType(req.Provider) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"requestId": requestID, "result": "error",
			"errorCode": "E_INVALID_PROVIDER",
			"message":   fmt.Sprintf("invalid provider: %s; must be one of telegram, discord, feishu, dummy", req.Provider),
		})
		return
	}
	cfg := setup.Configure(ProviderType(req.Provider), req.Token, req.WebhookSecret)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"message":   fmt.Sprintf("provider %s configured", req.Provider),
		"provider": map[string]interface{}{
			"provider":      cfg.Provider,
			"configured_at": cfg.ConfiguredAt,
		},
	})
}

func handleSetupGet(w http.ResponseWriter, r *http.Request, requestID string, setup *SetupStore) {
	redacted := setup.GetRedacted()
	resp := map[string]interface{}{
		"requestId":  requestID,
		"result":     "ok",
		"configured": setup.IsConfigured(),
		"provider":   redacted,
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

type apiErr struct {
	code string
	msg  string
}

func gatewayErrBody(code, msg string) map[string]interface{} {
	return map[string]interface{}{
		"result":    "error",
		"errorCode": code,
		"message":   msg,
	}
}

func checkGatewayToken(r *http.Request, expected string) *apiErr {
	if expected == "" {
		return nil
	}
	auth := r.Header.Get("Authorization")
	provided := ""
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		provided = strings.TrimSpace(after)
	}
	if provided == "" {
		return &apiErr{code: "E_GATEWAY_AUTH_REQUIRED", msg: "gateway api token required"}
	}
	left := sha256.Sum256([]byte(provided))
	right := sha256.Sum256([]byte(expected))
	if subtle.ConstantTimeCompare(left[:], right[:]) != 1 {
		return &apiErr{code: "E_GATEWAY_AUTH_INVALID", msg: "invalid gateway api token"}
	}
	return nil
}

func validateCommandAuth(input, sessionToken string, sessions *SessionStore) map[string]interface{} {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) < 4 {
		return nil // let command parser return E_PARSE
	}
	provider, chatID, requestID, cmdName := fields[0], fields[1], fields[2], fields[3]
	// /pair doesn't require auth
	if cmdName == "/pair" {
		return nil
	}
	session := sessions.GetSession(provider, chatID)
	if session == nil {
		return map[string]interface{}{
			"requestId": requestID, "result": "error",
			"errorCode": "E_SESSION_REQUIRED",
			"message":   "chat is not paired; run /pair <code> first",
		}
	}
	if sessionToken == "" {
		return map[string]interface{}{
			"requestId": requestID, "result": "error",
			"errorCode": "E_AUTH_REQUIRED",
			"message":   "session token required (provide via Authorization header or sessionToken field)",
		}
	}
	if session.SessionToken != sessionToken {
		return map[string]interface{}{
			"requestId": requestID, "result": "error",
			"errorCode": "E_AUTH_INVALID",
			"message":   "invalid session token",
		}
	}
	return nil
}

func parseCommandRequest(r *http.Request, maxBodyBytes int) (input, sessionToken string, status int, err *apiErr) {
	body, readErr := readBodyWithLimit(r, maxBodyBytes)
	if readErr != nil {
		return "", "", http.StatusRequestEntityTooLarge, &apiErr{code: "E_PAYLOAD_TOO_LARGE", msg: readErr.Error()}
	}

	// Session token from header
	if tok := strings.TrimSpace(r.Header.Get("X-Session-Token")); tok != "" {
		sessionToken = tok
	}

	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		var payload struct {
			Input        string `json:"input"`
			SessionToken string `json:"sessionToken"`
		}
		if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
			if strings.TrimSpace(payload.Input) != "" {
				input = strings.TrimSpace(payload.Input)
			}
			if sessionToken == "" && strings.TrimSpace(payload.SessionToken) != "" {
				sessionToken = strings.TrimSpace(payload.SessionToken)
			}
		}
		return input, sessionToken, http.StatusOK, nil
	}

	// Plain text
	raw := strings.TrimSpace(string(body))
	if raw != "" {
		input = raw
	}
	return input, sessionToken, http.StatusOK, nil
}

func readBodyWithLimit(r *http.Request, maxBytes int) ([]byte, error) {
	if cl := r.Header.Get("Content-Length"); cl != "" {
		n, err := strconv.Atoi(cl)
		if err == nil && n > maxBytes {
			return nil, fmt.Errorf("request body exceeds %d bytes", maxBytes)
		}
	}
	limited := io.LimitReader(r.Body, int64(maxBytes+1))
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, _ := json.Marshal(v)
	_, _ = w.Write(append(data, '\n'))
}

// newGatewayHTTPServer creates the HTTP server.
func newGatewayHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: gatewayReadHeaderTimeout,
		ReadTimeout:       gatewayReadTimeout,
		WriteTimeout:      gatewayWriteTimeout,
		IdleTimeout:       gatewayIdleTimeout,
	}
}

// isLoopbackGateway checks if a hostname is loopback-only.
func isLoopbackGateway(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// end of server.go
