package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"carrier/daemon/internal/lifecycle"
)

type issuePairCodeRequest struct {
	Code       string `json:"code"`
	TTLSeconds int    `json:"ttlSeconds"`
}

type verifyConsumeRequest struct {
	Code string `json:"code"`
}

type createDiagnosisHandoffRequest struct {
	AgentID   string `json:"agentId"`
	Consent   bool   `json:"consent"`
	Actor     string `json:"actor"`
	RequestID string `json:"requestId"`
}

func (s *Server) handleIssuePairCode(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/pairing/codes" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		codes := s.pairing.List()
		writeJSON(w, http.StatusOK, map[string]any{"codes": codes})
		return
	}
	if !allowMethod(w, r, http.MethodPost) {
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

func (s *Server) handleCreateDiagnosisHandoff(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	if r.URL.Path != "/api/v1/diagnosis/handoffs" {
		http.NotFound(w, r)
		return
	}

	var req createDiagnosisHandoffRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "E_USAGE", "agentId is required")
		return
	}

	handoff, err := s.lifecycle.CreateRemoteDiagnosisHandoff(req.AgentID, req.Consent, req.Actor, req.RequestID)
	if err != nil {
		status, code, message := mapLifecycleError(err)
		writeError(w, status, code, message)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          handoff.ID,
		"agentId":     handoff.AgentID,
		"consent":     handoff.Consent,
		"artifactRef": handoff.ArtifactRef,
		"status":      handoff.Status,
		"createdAt":   handoff.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
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
	if !validAgentIDPattern.MatchString(decoded) {
		return "", "", false
	}
	return decoded, action, true
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
	const maxJSONBodyBytes = 1 << 20

	if r.ContentLength > maxJSONBodyBytes {
		return fmt.Errorf("request body too large: max %d bytes", maxJSONBodyBytes)
	}

	body := io.LimitReader(r.Body, maxJSONBodyBytes+1)
	defer r.Body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(raw) > maxJSONBodyBytes {
		return fmt.Errorf("request body too large: max %d bytes", maxJSONBodyBytes)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	var trailing struct{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid json: trailing content after first json value")
	}

	return nil
}

func readStartOptions(r *http.Request) (lifecycle.StartOptions, error) {
	var body struct {
		Isolation bool `json:"isolation,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		return lifecycle.StartOptions{}, err
	}
	return lifecycle.StartOptions{
		Isolation: body.Isolation,
	}, nil
}

func readInstallOptions(r *http.Request) (lifecycle.InstallOptions, error) {
	var body struct {
		Isolation bool `json:"isolation,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		return lifecycle.InstallOptions{}, err
	}
	return lifecycle.InstallOptions{
		Isolation: body.Isolation,
	}, nil
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
