package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"carrier/daemon/internal/lifecycle"
)

type Server struct {
	lifecycle *lifecycle.Service
	pairing   *PairingCodeStore
}

type ServerOption func(*Server)

func WithPairingCodeStore(store *PairingCodeStore) ServerOption {
	return func(s *Server) {
		if store != nil {
			s.pairing = store
		}
	}
}

func NewServer(svc *lifecycle.Service, opts ...ServerOption) *Server {
	server := &Server{
		lifecycle: svc,
		pairing:   NewPairingCodeStore(nil),
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *Server) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents", s.handleListAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgentAction)
	mux.HandleFunc("/api/v1/pairing/codes", s.handleIssuePairCode)
	mux.HandleFunc("/api/v1/pairing/verify-consume", s.handleVerifyConsumePairCode)
	return mux
}

type daemonAgent struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Installed            bool   `json:"installed"`
	RuntimeState         string `json:"runtimeState"`
	Health               string `json:"health"`
	NeedsRemoteDiagnosis bool   `json:"needsRemoteDiagnosis"`
	LastError            string `json:"lastError,omitempty"`
	UpdatedAt            string `json:"updatedAt"`
}

type listAgentsResponse struct {
	Agents []daemonAgent `json:"agents"`
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if r.URL.Path != "/api/v1/agents" {
		http.NotFound(w, r)
		return
	}

	states := s.lifecycle.ListAgents()
	resp := listAgentsResponse{
		Agents: make([]daemonAgent, 0, len(states)),
	}
	for _, state := range states {
		resp.Agents = append(resp.Agents, daemonAgent{
			ID:                   state.ID,
			Name:                 s.lifecycle.AgentName(state.ID),
			Version:              state.Version,
			Installed:            state.Install == lifecycle.InstallStateInstalled,
			RuntimeState:         string(state.Runtime),
			Health:               string(state.Health),
			NeedsRemoteDiagnosis: state.NeedsRemoteDiagnosis,
			LastError:            state.LastError,
			UpdatedAt:            state.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAgentAction(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}

	agentID, action, ok := parseAgentActionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "stop":
		if err := s.lifecycle.Stop(r.Context(), agentID); err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"agentId": agentID,
			"stopped": true,
		})
	case "upgrade":
		result, err := s.lifecycle.Upgrade(r.Context(), agentID)
		if err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		resp := map[string]any{
			"agentId":     result.AgentID,
			"fromVersion": result.FromVersion,
			"toVersion":   result.ToVersion,
			"backupPath":  result.BackupPath,
		}
		if strings.TrimSpace(result.BackupPath) != "" {
			resp["rollbackHint"] = fmt.Sprintf("restore from %s before retrying upgrade", result.BackupPath)
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		http.NotFound(w, r)
	}
}

type issuePairCodeRequest struct {
	Code       string `json:"code"`
	TTLSeconds int    `json:"ttlSeconds"`
}

func (s *Server) handleIssuePairCode(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	if r.URL.Path != "/api/v1/pairing/codes" {
		http.NotFound(w, r)
		return
	}

	req := issuePairCodeRequest{}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	var (
		record PairingCodeRecord
		err    error
	)
	if strings.TrimSpace(req.Code) == "" {
		record, err = s.pairing.Issue(ttl)
	} else {
		record, err = s.pairing.Register(req.Code, ttl)
	}
	if err != nil {
		if errors.Is(err, ErrPairCodeRequired) {
			writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "E_INTERNAL", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, record)
}

type verifyConsumeRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleVerifyConsumePairCode(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	if r.URL.Path != "/api/v1/pairing/verify-consume" {
		http.NotFound(w, r)
		return
	}

	req := verifyConsumeRequest{}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
		return
	}

	if err := s.pairing.VerifyAndConsume(req.Code); err != nil {
		if errors.Is(err, ErrPairCodeInvalid) {
			writeError(w, http.StatusBadRequest, "E_PAIR_CODE_INVALID", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "E_INTERNAL", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":     req.Code,
		"consumed": true,
	})
}

func parseAgentActionPath(path string) (agentID string, action string, ok bool) {
	const prefix = "/api/v1/agents/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	if strings.Contains(rest, "//") {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "E_USAGE", "method not allowed")
	return false
}

func readJSON(r *http.Request, dst any) error {
	body := io.LimitReader(r.Body, 1<<20)
	defer r.Body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
