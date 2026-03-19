package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"carrier/daemon/internal/lifecycle"
	"carrier/shared/redact"
)

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
		resp.Agents = append(resp.Agents, s.agentFromState(state))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMergedLogs(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if r.URL.Path != "/api/v1/logs" {
		http.NotFound(w, r)
		return
	}

	tail := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("tail")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			tail = parsed
			if tail > maxTailParam {
				tail = maxTailParam
			}
		}
	}
	lines := s.lifecycle.MergedLogs(tail)
	redactedLines := make([]string, len(lines))
	for i, line := range lines {
		redactedLines[i] = redact.RedactText(line)
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": redactedLines, "truncated": false})
}

type auditLogRecord struct {
	RequestID string `json:"requestId"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Result    string `json:"result"`
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}

const (
	defaultAuditQueryLimit = 200
	maxAuditQueryLimit     = 1000
	// maxTailParam caps the tail query parameter to prevent excessive memory
	// allocation from adversarial input (see issue #589).
	maxTailParam = 10000
)

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	if r.URL.Path != "/api/v1/audit/logs" {
		http.NotFound(w, r)
		return
	}

	actorFilter := strings.TrimSpace(r.URL.Query().Get("actor"))
	actionFilter := strings.TrimSpace(r.URL.Query().Get("action"))
	requestIDFilter := strings.TrimSpace(r.URL.Query().Get("request_id"))
	resultFilterRaw := strings.TrimSpace(r.URL.Query().Get("result"))

	var resultFilter lifecycle.AuditResult
	if resultFilterRaw != "" {
		switch resultFilterRaw {
		case string(lifecycle.AuditResultSuccess):
			resultFilter = lifecycle.AuditResultSuccess
		case string(lifecycle.AuditResultFailure):
			resultFilter = lifecycle.AuditResultFailure
		case string(lifecycle.AuditResultNeutral):
			resultFilter = lifecycle.AuditResultNeutral
		default:
			writeError(w, http.StatusBadRequest, "E_USAGE", "result must be one of success|failure|neutral")
			return
		}
	}

	limit := defaultAuditQueryLimit
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "E_USAGE", "limit must be a positive integer")
			return
		}
		if parsed > maxAuditQueryLimit {
			limit = maxAuditQueryLimit
		} else {
			limit = parsed
		}
	}

	source := s.lifecycle.AuditLogs()
	filtered := make([]auditLogRecord, 0, len(source))
	for _, entry := range source {
		if actorFilter != "" && entry.Actor != actorFilter {
			continue
		}
		if actionFilter != "" && entry.Action != actionFilter {
			continue
		}
		if requestIDFilter != "" && entry.RequestID != requestIDFilter {
			continue
		}
		if resultFilterRaw != "" && entry.Result != resultFilter {
			continue
		}
		filtered = append(filtered, auditLogRecord{
			RequestID: entry.RequestID,
			Actor:     entry.Actor,
			Action:    entry.Action,
			Target:    entry.Target,
			Result:    string(entry.Result),
			ErrorCode: entry.ErrorCode,
			Message:   redact.RedactText(entry.Message),
			Timestamp: entry.Timestamp.UTC().Format(time.RFC3339Nano),
		})
	}

	total := len(filtered)
	if total > limit {
		filtered = filtered[total-limit:]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"auditLogs": filtered,
		"total":     total,
	})
}

func (s *Server) handleAgentAction(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/agents/status" {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		states := s.lifecycle.ListAgents()
		resp := struct {
			Statuses []daemonAgent `json:"statuses"`
		}{Statuses: make([]daemonAgent, 0, len(states))}
		for _, state := range states {
			resp.Statuses = append(resp.Statuses, s.agentFromState(state))
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	agentID, action, ok := parseAgentActionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "install":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		installOpts, err := readInstallOptions(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
			return
		}
		// Detach long-running install execution from client disconnects/timeouts.
		// The lifecycle layer enforces command timeout independently.
		if err := s.lifecycle.InstallWithOptions(context.WithoutCancel(r.Context()), agentID, installOpts); err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agentId": agentID, "installed": true})
	case "start":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		startOpts, err := readStartOptions(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "E_USAGE", err.Error())
			return
		}
		if err := s.lifecycle.StartWithOptions(r.Context(), agentID, startOpts); err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agentId": agentID, "started": true})
	case "stop":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		if err := s.lifecycle.Stop(r.Context(), agentID); err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agentId": agentID, "stopped": true})
	case "status":
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		state, err := s.lifecycle.Status(agentID)
		if err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"statuses": []daemonAgent{s.agentFromState(state)},
		})
	case "logs":
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		tail := 200
		if raw := strings.TrimSpace(r.URL.Query().Get("tail")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				tail = parsed
				if tail > maxTailParam {
					tail = maxTailParam
				}
			}
		}
		lines, err := s.lifecycle.Logs(agentID, tail)
		if err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		redactedLines := make([]string, len(lines))
		for i, line := range lines {
			redactedLines[i] = redact.RedactText(line)
		}
		writeJSON(w, http.StatusOK, map[string]any{"lines": redactedLines, "truncated": false})
	case "diagnose":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		artifactRef, err := s.lifecycle.Diagnose(agentID)
		if err != nil {
			status, code, message := mapLifecycleError(err)
			writeError(w, status, code, message)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"artifactRef": artifactRef})
	case "upgrade":
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		// Upgrade can take minutes and should continue even if the HTTP client
		// connection is interrupted.
		result, err := s.lifecycle.Upgrade(context.WithoutCancel(r.Context()), agentID)
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
