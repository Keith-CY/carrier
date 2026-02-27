package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

func handleRemoteHosts(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}

	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/remote/hosts"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			hosts, err := listRemoteHosts()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote hosts", "load remote hosts", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "hosts": hosts})
			return
		case http.MethodPost:
			var req RemoteHost
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			host, err := upsertRemoteHost(req)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "host": host})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	parts := strings.Split(trimmed, "/")
	hostID := strings.TrimSpace(parts[0])
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "host id is required"))
		return
	}
	host, ok, err := getRemoteHost(hostID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote host", "get remote host", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", hostID)))
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var patch RemoteHost
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			updated, err := patchRemoteHost(hostID, patch)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "host": updated})
			return
		case http.MethodDelete:
			deleted, err := deleteRemoteHost(hostID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to delete remote host", "delete remote host", err)
				return
			}
			if !deleted {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", hostID)))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "deleted": true})
			return
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "host": host})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	sub := strings.ToLower(strings.TrimSpace(parts[1]))
	switch sub {
	case "check":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		checkResult, checkErr := checkRemoteHostAndMaybeRepair(ctx, host)
		recordRemoteOperationMetric(remoteOpHostCheck, startedAt, checkErr)
		if checkErr != nil {
			_ = updateRemoteHostHealth(hostID, RemoteHealthUnhealthy, checkErr.Error())
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CHECK_FAILED", checkErr.Error()))
			return
		}
		health := RemoteHealthUnhealthy
		if checkResult.SSHOK && checkResult.OpenClawFound && checkResult.GatewayHealthy {
			health = RemoteHealthHealthy
		}
		_ = updateRemoteHostHealth(hostID, health, strings.Join(checkResult.Details, "; "))
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "check": checkResult})
		return
	case "instances":
		handleRemoteHostInstances(w, r, requestID, hostID, host, parts)
		return
	case "config":
		handleRemoteHostConfig(w, r, requestID, host)
		return
	case "sessions":
		handleRemoteHostSessions(w, r, requestID, host, parts)
		return
	case "memory":
		handleRemoteHostMemory(w, r, requestID, host)
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported remote host action"))
		return
	}
}

func handleRemoteMetrics(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"metrics":   remoteMetrics.snapshot(),
	})
}

func handleRemoteSSHConfigHosts(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	hosts, err := listLocalSSHConfigHosts()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load local ssh config hosts", "load local ssh config hosts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"hosts":     hosts,
	})
}

func handleRemoteHostInstances(w http.ResponseWriter, r *http.Request, requestID, hostID string, host RemoteHost, parts []string) {
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		instances, steps, err := remoteListInstancesForHost(ctx, host, hostID)
		recordRemoteOperationMetric(remoteOpInstancesList, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_LIST_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "instances": instances, "steps": steps})
		return
	}
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "instance action path is required"))
		return
	}
	agentID := strings.TrimSpace(parts[2])
	action := strings.ToLower(strings.TrimSpace(parts[3]))
	if err := validateAgentIdentifier(agentID); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}

	switch action {
	case "status":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		status, steps, err := remoteGetInstanceStatus(ctx, host, hostID, agentID)
		recordRemoteOperationMetric(remoteOpInstancesStatus, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_STATUS_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "instance": status, "steps": steps})
		return
	case "install":
		if len(parts) >= 5 {
			subAction := strings.ToLower(strings.TrimSpace(parts[4]))
			if subAction != "stream" {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported remote install action"))
				return
			}
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
				return
			}
			streamRemoteInstallResponse(w, r, requestID, hostID, host, agentID)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Minute)
		defer cancel()
		installResult, err := remoteInstallOpenClaw(ctx, host, hostID, agentID)
		recordRemoteOperationMetric(remoteOpInstancesInstall, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_INSTALL_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "install": installResult})
		return
	case "repair":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		repairResult, err := remoteRepairOpenClaw(ctx, host, hostID, agentID)
		recordRemoteOperationMetric(remoteOpInstancesRepair, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_REPAIR_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "repair": repairResult})
		return
	case "logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		tail := parsePositiveInt(r.URL.Query().Get("tail"), 200)
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		logs, steps, err := remoteGetLogs(ctx, host, agentID, tail)
		recordRemoteOperationMetric(remoteOpInstancesLogs, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_LOGS_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "logs": logs, "steps": steps})
		return
	case "sync":
		if len(parts) >= 5 {
			subAction := strings.ToLower(strings.TrimSpace(parts[4]))
			if subAction != "status" {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported remote sync action"))
				return
			}
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
				return
			}
			status, err := remoteGetInstanceSyncStatus(hostID, agentID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load sync status", "get sync status", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "status": status})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeInternalGatewayError(w, http.StatusBadRequest, "E_BAD_REQUEST", "invalid request body", "decoding sync request", err)
            return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		syncResult, steps, err := remoteSyncInstanceConfig(ctx, host, hostID, agentID, req.Mode)
		recordRemoteOperationMetric(remoteOpInstancesSync, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_instance_sync", hostID+":"+agentID, "failure", map[string]interface{}{
				"mode":  req.Mode,
				"error": err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_SYNC_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_instance_sync", hostID+":"+agentID, "success", map[string]interface{}{
			"mode":           syncResult.Mode,
			"driftState":     syncResult.DriftState,
			"lastRemoteHash": syncResult.LastRemoteHash,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"sync":      syncResult,
			"steps":     steps,
		})
		return
	case "diagnose":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		diagnoseResult, steps, err := remoteDiagnoseInstanceConfig(ctx, host, hostID, agentID)
		recordRemoteOperationMetric(remoteOpInstancesDiagnose, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_instance_diagnose", hostID+":"+agentID, "failure", map[string]interface{}{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_DIAGNOSE_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_instance_diagnose", hostID+":"+agentID, "success", map[string]interface{}{
			"result":     diagnoseResult.Result,
			"driftState": diagnoseResult.DriftState,
			"remoteHash": diagnoseResult.LastRemoteHash,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"diagnose":  diagnoseResult,
			"steps":     steps,
		})
		return
	case "reconcile":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		reconcileResult, steps, err := remoteReconcileInstanceConfig(ctx, host, hostID, agentID)
		recordRemoteOperationMetric(remoteOpInstancesReconcile, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_instance_reconcile", hostID+":"+agentID, "failure", map[string]interface{}{
				"error": err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_RECONCILE_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_instance_reconcile", hostID+":"+agentID, "success", map[string]interface{}{
			"driftState": reconcileResult.DriftState,
			"reconciled": reconcileResult.Reconciled,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"reconcile": reconcileResult,
			"steps":     steps,
		})
		return
	case "rollback":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Commit string `json:"commit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid request body: "+err.Error()))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		rollbackResult, steps, err := remoteRollbackInstanceConfig(ctx, host, hostID, agentID, req.Commit)
		recordRemoteOperationMetric(remoteOpInstancesRollback, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_instance_rollback", hostID+":"+agentID, "failure", map[string]interface{}{
				"commit": req.Commit,
				"error":  err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_ROLLBACK_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_instance_rollback", hostID+":"+agentID, "success", map[string]interface{}{
			"fromCommit": rollbackResult.FromCommit,
			"newCommit":  rollbackResult.NewCommit,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"rollback":  rollbackResult,
			"steps":     steps,
		})
		return
	case "codeagent":
		handleRemoteCodeAgent(w, r, requestID, host, hostID, agentID, parts)
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported remote instance action"))
		return
	}
}

func streamRemoteInstallResponse(w http.ResponseWriter, r *http.Request, requestID, hostID string, host RemoteHost, agentID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var writeMu sync.Mutex
	emit := func(payload map[string]interface{}) bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := writeSSEEvent(w, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !emit(map[string]interface{}{
		"type":      "start",
		"requestId": requestID,
		"hostId":    hostID,
		"agentId":   agentID,
	}) {
		return
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Minute)
	defer cancel()

	installResult, installErr := remoteInstallOpenClawStreaming(ctx, host, hostID, agentID, func(chunk remoteStreamChunk) {
		line := strings.TrimSpace(RedactErrorMessage(chunk.Text))
		if line == "" {
			return
		}
		ok := emit(map[string]interface{}{
			"type":   "log",
			"stream": chunk.Stream,
			"line":   line,
		})
		if !ok {
			cancel()
		}
	})
	recordRemoteOperationMetric(remoteOpInstancesInstall, startedAt, installErr)

	if installResult != nil {
		_ = emit(map[string]interface{}{
			"type":    "result",
			"install": installResult,
		})
	}
	if installErr != nil {
		_ = emit(map[string]interface{}{
			"type":      "error",
			"errorCode": "E_REMOTE_INSTALL_FAILED",
			"message":   RedactErrorMessage(installErr.Error()),
		})
		_ = emit(map[string]interface{}{
			"type":         "finish",
			"finishReason": "error",
		})
		return
	}
	_ = emit(map[string]interface{}{
		"type":         "finish",
		"finishReason": "stop",
	})
}

func handleRemoteHostConfig(w http.ResponseWriter, r *http.Request, requestID string, host RemoteHost) {
	switch r.Method {
	case http.MethodGet:
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cfg, _, steps, err := remoteReadConfig(ctx, host)
		recordRemoteOperationMetric(remoteOpConfigRead, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CONFIG_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "config": cfg, "steps": steps})
		return
	case http.MethodPatch:
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		patch := body
		if candidate, ok := body["patch"].(map[string]interface{}); ok {
			patch = candidate
		}
		if len(patch) == 0 {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "patch payload cannot be empty"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		merged, snapshot, steps, err := remotePatchConfig(ctx, host, patch)
		recordRemoteOperationMetric(remoteOpConfigPatch, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CONFIG_PATCH_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"config":    merged,
			"snapshot":  snapshot,
			"steps":     steps,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
}

func handleRemoteHostSessions(w http.ResponseWriter, r *http.Request, requestID string, host RemoteHost, parts []string) {
	agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
	if agentID == "" {
		agentID = "main"
	}
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		sessions, steps, err := remoteListSessions(ctx, host, agentID)
		recordRemoteOperationMetric(remoteOpSessionsList, startedAt, err)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_SESSIONS_FAILED", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "sessions": sessions, "steps": steps})
		return
	}
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "session action path is required"))
		return
	}
	sessionID := strings.TrimSpace(parts[2])
	normalizedSessionID, sessionErr := validateRemoteSessionIdentifier(sessionID)
	if sessionErr != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", sessionErr.Error()))
		return
	}
	sessionID = normalizedSessionID
	action := strings.ToLower(strings.TrimSpace(parts[3]))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var steps []remoteExecResult
	var err error
	var opClass string
	startedAt := time.Now()
	switch action {
	case "archive":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		opClass = remoteOpSessionArchive
		steps, err = remoteArchiveSession(ctx, host, agentID, sessionID)
	case "delete":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		opClass = remoteOpSessionDelete
		steps, err = remoteDeleteSession(ctx, host, agentID, sessionID)
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported session action"))
		return
	}
	recordRemoteOperationMetric(opClass, startedAt, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_SESSION_ACTION_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "sessionId": sessionID, "action": action, "steps": steps})
}

func handleRemoteHostMemory(w http.ResponseWriter, r *http.Request, requestID string, host RemoteHost) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
	if agentID == "" {
		agentID = "main"
	}
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	entries, steps, err := remoteListMemory(ctx, host, agentID)
	recordRemoteOperationMetric(remoteOpMemoryList, startedAt, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_MEMORY_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "memory": entries, "steps": steps})
}

func handleProviderProfiles(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/provider-profiles"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			profiles, err := listProviderProfiles()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list provider profiles", "list provider profiles", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "profiles": profiles})
			return
		case http.MethodPost:
			var req struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Provider string `json:"provider"`
				Model    string `json:"model"`
				BaseURL  string `json:"baseUrl"`
				AuthRef  string `json:"authRef"`
				Enabled  *bool  `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			enabled := true
			if req.Enabled != nil {
				enabled = *req.Enabled
			}
			profile, err := upsertProviderProfile(ProviderProfile{
				ID:       strings.TrimSpace(req.ID),
				Name:     strings.TrimSpace(req.Name),
				Provider: strings.TrimSpace(req.Provider),
				Model:    strings.TrimSpace(req.Model),
				BaseURL:  strings.TrimSpace(req.BaseURL),
				AuthRef:  strings.TrimSpace(req.AuthRef),
				Enabled:  enabled,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "profile": profile})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}
	parts := strings.Split(trimmed, "/")
	profileID := strings.TrimSpace(parts[0])
	if profileID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "profile id is required"))
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				Name     *string `json:"name"`
				Provider *string `json:"provider"`
				Model    *string `json:"model"`
				BaseURL  *string `json:"baseUrl"`
				AuthRef  *string `json:"authRef"`
				Enabled  *bool   `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			updated, err := patchProviderProfile(profileID, providerProfilePatch{
				Name:     req.Name,
				Provider: req.Provider,
				Model:    req.Model,
				BaseURL:  req.BaseURL,
				AuthRef:  req.AuthRef,
				Enabled:  req.Enabled,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "profile": updated})
			return
		case http.MethodDelete:
			deleted, err := deleteProviderProfile(profileID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to delete provider profile", "delete provider profile", err)
				return
			}
			if !deleted {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_PROFILE_NOT_FOUND", fmt.Sprintf("profile %s not found", profileID)))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "deleted": true})
			return
		case http.MethodGet:
			profile, found, err := getProviderProfile(profileID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load provider profile", "get provider profile", err)
				return
			}
			if !found {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_PROFILE_NOT_FOUND", fmt.Sprintf("profile %s not found", profileID)))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "profile": profile})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}
	if len(parts) == 2 && strings.EqualFold(parts[1], "test") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		profile, found, err := getProviderProfile(profileID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load provider profile", "get provider profile", err)
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_PROFILE_NOT_FOUND", fmt.Sprintf("profile %s not found", profileID)))
			return
		}
		var req struct {
			HostID string `json:"hostId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeInternalGatewayError(w, http.StatusBadRequest, "E_BAD_REQUEST", "invalid request body", "decoding profile-test request", err)
            return
		}
		if strings.TrimSpace(req.HostID) == "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"test": map[string]interface{}{
					"ok":       true,
					"provider": profile.Provider,
					"model":    profile.Model,
					"mode":     "local_validation",
				},
			})
			return
		}
		host, found, err := getRemoteHost(req.HostID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote host", "get remote host", err)
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", req.HostID)))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		startedAt := time.Now()
		check, checkErr := checkRemoteHostAndMaybeRepair(ctx, host)
		recordRemoteOperationMetric(remoteOpProfileTestRemote, startedAt, checkErr)
		if checkErr != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_PROFILE_TEST_FAILED", checkErr.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"test": map[string]interface{}{
				"ok":               check.SSHOK && check.OpenClawFound,
				"provider":         profile.Provider,
				"model":            profile.Model,
				"hostId":           req.HostID,
				"openclawDetected": check.OpenClawFound,
				"gatewayHealthy":   check.GatewayHealthy,
			},
		})
		return
	}

	writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported provider profile action"))
}

func handleProviderBindings(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/provider-bindings"), "/")
	if trimmed != "" {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		deleted, err := deleteProviderBinding(trimmed)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to delete provider binding", "delete provider binding", err)
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_BINDING_NOT_FOUND", fmt.Sprintf("binding %s not found", trimmed)))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "deleted": true})
		return
	}

	switch r.Method {
	case http.MethodGet:
		bindings, err := listProviderBindings()
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list provider bindings", "list provider bindings", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"requestId": requestID, "result": "ok", "bindings": bindings})
		return
	case http.MethodPost:
		if !flags.ProviderBindingEnabled {
			writeJSON(w, http.StatusForbidden, gatewayErrBody("E_FEATURE_DISABLED", "provider binding feature is disabled"))
			return
		}
		var req ProviderBinding
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		req = normalizeProviderBinding(req)
		if err := validateProviderBinding(req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		profile, found, err := getProviderProfile(req.ProfileID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load provider profile", "get provider profile", err)
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_PROFILE_NOT_FOUND", fmt.Sprintf("profile %s not found", req.ProfileID)))
			return
		}
		var host RemoteHost
		var hostID string
		targetAgentID := ""
		switch req.TargetType {
		case "host":
			hostID = req.TargetID
		case "instance":
			resolvedHostID, resolvedAgentID := splitInstanceBindingTarget(req.TargetID)
			if resolvedHostID == "" || resolvedAgentID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "instance targetId must be in format <hostId>:<agentId>"))
				return
			}
			hostID = resolvedHostID
			targetAgentID = resolvedAgentID
		}
		host, found, err = getRemoteHost(hostID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote host", "get remote host", err)
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", hostID)))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		startedAt := time.Now()
		_, snapshot, steps, applyErr := applyProviderProfileToRemote(ctx, host, profile, targetAgentID)
		recordRemoteOperationMetric(remoteOpProviderBindingApply, startedAt, applyErr)
		if applyErr != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_PROVIDER_BIND_APPLY_FAILED", applyErr.Error()))
			return
		}
		binding, err := upsertProviderBinding(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"binding":   binding,
			"snapshot":  snapshot,
			"steps":     steps,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
}

func splitInstanceBindingTarget(targetID string) (string, string) {
	trimmed := strings.TrimSpace(targetID)
	if trimmed == "" {
		return "", ""
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.SplitN(trimmed, "/", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}

type remoteChatRequest struct {
	HostID    string `json:"hostId"`
	AgentID   string `json:"agentId"`
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
}

type unifiedChatStreamRequest struct {
	Target    string `json:"target,omitempty"`
	HostID    string `json:"hostId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
	Provider  string `json:"provider,omitempty"`
	ChatID    string `json:"chatId,omitempty"`
}

func handleUnifiedChatStream(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, daemon *DaemonClient) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	var req unifiedChatStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		target = "remote"
	}

	switch target {
	case "remote":
		flags := effectiveGatewayFeatureFlags(cfg)
		if !flags.RemoteChatEnabled {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote chat is disabled"))
			return
		}
		remoteReq := remoteChatRequest{
			HostID:    strings.TrimSpace(req.HostID),
			AgentID:   strings.TrimSpace(req.AgentID),
			Message:   strings.TrimSpace(req.Message),
			SessionID: strings.TrimSpace(req.SessionID),
		}
		streamRemoteChatResponse(w, r, requestID, remoteReq)
		return
	case "local":
		localReq := unifiedChatStreamRequest{
			Target:    target,
			AgentID:   strings.TrimSpace(req.AgentID),
			Message:   strings.TrimSpace(req.Message),
			SessionID: strings.TrimSpace(req.SessionID),
			Provider:  strings.TrimSpace(req.Provider),
			ChatID:    strings.TrimSpace(req.ChatID),
		}
		streamLocalChatResponse(w, r, requestID, daemon, localReq)
		return
	default:
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "target must be local or remote"))
		return
	}
}

func handleRemoteChatStream(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteChatEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote chat is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	var req remoteChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	req.HostID = strings.TrimSpace(req.HostID)
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.Message = strings.TrimSpace(req.Message)
	req.SessionID = strings.TrimSpace(req.SessionID)
	streamRemoteChatResponse(w, r, requestID, req)
}

func streamRemoteChatResponse(w http.ResponseWriter, r *http.Request, requestID string, req remoteChatRequest) {
	if req.HostID == "" || req.AgentID == "" || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "hostId, agentId and message are required"))
		return
	}
	host, found, err := getRemoteHost(req.HostID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote host", "get remote host", err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", req.HostID)))
		return
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	_, _, _, healthErr := ensureRemoteHealthyForOperation(ctx, host)
	if healthErr != nil {
		recordRemoteOperationMetric(remoteOpRemoteChatStream, startedAt, healthErr)
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CHAT_PRECHECK_FAILED", healthErr.Error()))
		return
	}
	payload, _, chatErr := remoteChatViaOpenClaw(ctx, host, req.AgentID, req.Message, req.SessionID)
	recordRemoteOperationMetric(remoteOpRemoteChatStream, startedAt, chatErr)
	if chatErr != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CHAT_FAILED", chatErr.Error()))
		return
	}
	sessionID := strings.TrimSpace(anyToString(payload["sessionId"]))
	streamChatSSE(w, requestID, payload, sessionID)
}

func streamLocalChatResponse(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient, req unifiedChatStreamRequest) {
	if daemon == nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "daemon client is not configured"))
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "message is required"))
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "webui"
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		chatID = sessionID
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	payload := map[string]interface{}{}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID != "" {
		chatResult, err := daemon.ChatAgent(
			ctx,
			agentID,
			req.Message,
			sessionID,
			"webui:local-chat",
			requestID,
		)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_LOCAL_CHAT_FAILED", err.Error()))
			return
		}
		payload["message"] = strings.TrimSpace(chatResult.Message)
		payload["agentId"] = strings.TrimSpace(chatResult.AgentID)
		if strings.TrimSpace(chatResult.SessionID) != "" {
			sessionID = strings.TrimSpace(chatResult.SessionID)
		}
	} else {
		chatResult, err := daemon.ChatBaseAgent(
			ctx,
			provider,
			chatID,
			requestID,
			req.Message,
			"webui:local-chat",
		)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_LOCAL_CHAT_FAILED", err.Error()))
			return
		}
		payload["message"] = strings.TrimSpace(chatResult.Message)
		payload["action"] = strings.TrimSpace(chatResult.Action)
	}
	streamChatSSE(w, requestID, payload, sessionID)
}

func streamChatSSE(w http.ResponseWriter, requestID string, payload map[string]interface{}, sessionID string) {
	text := extractChatResponseText(payload)
	if text == "" {
		raw, _ := json.Marshal(payload)
		text = string(raw)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "streaming not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if err := writeSSEEvent(w, map[string]interface{}{"type": "start", "requestId": requestID}); err != nil {
		return
	}
	flusher.Flush()

	chunks := chunkString(text, 180)
	for _, chunk := range chunks {
		if err := writeSSEEvent(w, map[string]interface{}{"type": "text-delta", "delta": chunk}); err != nil {
			return
		}
		flusher.Flush()
	}
	if strings.TrimSpace(sessionID) != "" {
		if err := writeSSEEvent(w, map[string]interface{}{"type": "session", "sessionId": strings.TrimSpace(sessionID)}); err != nil {
			return
		}
		flusher.Flush()
	}
	if err := writeSSEEvent(w, map[string]interface{}{"type": "finish", "finishReason": "stop"}); err != nil {
		return
	}
	flusher.Flush()
}

func writeSSEEvent(w http.ResponseWriter, payload map[string]interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	return nil
}

func chunkString(input string, size int) []string {
	if size <= 0 {
		return []string{input}
	}
	runes := []rune(input)
	if len(runes) <= size {
		return []string{input}
	}
	chunks := make([]string, 0, len(runes)/size+1)
	for len(runes) > size {
		chunks = append(chunks, string(runes[:size]))
		runes = runes[size:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	if len(chunks) == 0 {
		chunks = append(chunks, "")
	}
	return chunks
}
