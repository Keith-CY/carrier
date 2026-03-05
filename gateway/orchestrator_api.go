package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultOrchestratorWorkerIdleTTL  = 10 * time.Minute
	defaultOrchestratorMaxConcurrency = 8
)

var orchestratorExecutionRunState = struct {
	mu      sync.Mutex
	running map[string]bool
}{
	running: map[string]bool{},
}

var (
	orchestratorListLeasesByExecution = listOrchestratorWorkerLeasesByExecution
	orchestratorInstallAgent          = remoteInstallAgent
)

func logOrchestratorPersistError(context string, err error) {
	if err == nil {
		return
	}
	log.Printf("[gateway] %s: %s", strings.TrimSpace(context), RedactErrorMessage(err.Error()))
}

type orchestratorWorkerPool struct {
	key string
	ch  chan OrchestratorWorkerLease
}

func handleOrchestratorExecutions(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/orchestrator/executions"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			executions, err := listOrchestratorExecutions()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator executions", "list orchestrator executions", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId":  requestID,
				"result":     "ok",
				"executions": executions,
			})
			return
		case http.MethodPost:
			var req OrchestratorExecution
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			normalized, err := normalizeOrchestratorExecution(req)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			if normalized.IdempotencyKey != "" {
				existing, ok, findErr := findOrchestratorExecutionByIdempotencyKey(normalized.IdempotencyKey)
				if findErr != nil {
					writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to lookup idempotency key", "find orchestrator execution by idempotency key", findErr)
					return
				}
				if ok {
					writeJSON(w, http.StatusOK, map[string]interface{}{
						"requestId": requestID,
						"result":    "ok",
						"execution": existing,
					})
					return
				}
			}

			now := nowTimestamp()
			normalized.ID = uuid.NewString()
			normalized.Status = OrchestratorExecutionStatusPendingAuthorization
			normalized.Authorization = OrchestratorAuthorization{}
			normalized.Results = []OrchestratorTaskResult{}
			normalized.CreatedAt = now
			normalized.UpdatedAt = now
			normalized.Error = ""

			saved, saveErr := upsertOrchestratorExecution(normalized)
			if saveErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to save orchestrator execution", "upsert orchestrator execution", saveErr)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": saved,
			})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	parts := strings.Split(trimmed, "/")
	executionID := strings.TrimSpace(parts[0])
	if executionID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "execution id is required"))
		return
	}
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load orchestrator execution", "get orchestrator execution", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", fmt.Sprintf("orchestrator execution %s not found", executionID)))
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		leases, leaseErr := orchestratorListLeasesByExecution(executionID)
		if leaseErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load orchestrator worker leases", "list orchestrator worker leases by execution", leaseErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": execution,
			"workers":   leases,
		})
		return
	}

	action := strings.ToLower(strings.TrimSpace(parts[1]))
	switch action {
	case "authorize":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Approved       bool   `json:"approved"`
			Actor          string `json:"actor"`
			MaxConcurrency int    `json:"maxConcurrency,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		actor := strings.TrimSpace(req.Actor)
		if actor == "" {
			actor = "operator"
		}
		if !req.Approved {
			execution.Authorization = OrchestratorAuthorization{
				InfrastructureApproved: false,
				ApprovedBy:             actor,
				ApprovedAt:             nowTimestamp(),
			}
			execution.Status = OrchestratorExecutionStatusDeclined
			execution.CompletedAt = nowTimestamp()
			execution.UpdatedAt = nowTimestamp()
			execution.Error = "infrastructure authorization declined"
			updated, saveErr := upsertOrchestratorExecution(execution)
			if saveErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update orchestrator execution", "upsert orchestrator execution declined", saveErr)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": updated,
			})
			return
		}

		if req.MaxConcurrency > 0 {
			execution.MaxConcurrency = req.MaxConcurrency
		}
		if execution.MaxConcurrency <= 0 {
			execution.MaxConcurrency = defaultOrchestratorMaxConcurrency
		}
		if execution.MaxConcurrency > 64 {
			execution.MaxConcurrency = 64
		}
		if execution.Status == OrchestratorExecutionStatusCompleted ||
			execution.Status == OrchestratorExecutionStatusFailed ||
			execution.Status == OrchestratorExecutionStatusDeclined {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": execution,
			})
			return
		}
		execution.Authorization = OrchestratorAuthorization{
			InfrastructureApproved: true,
			ApprovedBy:             actor,
			ApprovedAt:             nowTimestamp(),
		}
		if execution.StartedAt == "" {
			execution.StartedAt = nowTimestamp()
		}
		execution.Status = OrchestratorExecutionStatusProvisioning
		execution.UpdatedAt = nowTimestamp()
		execution.Error = ""
		updated, saveErr := upsertOrchestratorExecution(execution)
		if saveErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update orchestrator execution", "upsert orchestrator execution authorize", saveErr)
			return
		}
		startOrchestratorExecutionAsync(updated.ID)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": updated,
		})
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported orchestrator execution action"))
		return
	}
}

func handleOrchestratorWorkersReclaim(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	var req struct {
		Force      bool `json:"force"`
		IdleTTLSec int  `json:"idleTtlSec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	idleTTL := defaultOrchestratorWorkerIdleTTL
	if req.IdleTTLSec > 0 {
		idleTTL = time.Duration(req.IdleTTLSec) * time.Second
	}
	summary, err := reclaimOrchestratorWorkers(context.Background(), req.Force, idleTTL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_RECLAIM_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"reclaim":   summary,
	})
}

func startOrchestratorExecutionAsync(executionID string) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return
	}
	orchestratorExecutionRunState.mu.Lock()
	if orchestratorExecutionRunState.running[id] {
		orchestratorExecutionRunState.mu.Unlock()
		return
	}
	orchestratorExecutionRunState.running[id] = true
	orchestratorExecutionRunState.mu.Unlock()

	go func() {
		defer func() {
			orchestratorExecutionRunState.mu.Lock()
			delete(orchestratorExecutionRunState.running, id)
			orchestratorExecutionRunState.mu.Unlock()
		}()
		runOrchestratorExecution(id)
	}()
}

func runOrchestratorExecution(executionID string) {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil || !ok {
		return
	}
	if !execution.Authorization.InfrastructureApproved {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	leases, provisionErr := provisionOrchestratorWorkers(ctx, execution)
	if provisionErr != nil {
		markOrchestratorExecutionFailed(execution, provisionErr, nil)
		return
	}

	execution.Status = OrchestratorExecutionStatusRunning
	execution.UpdatedAt = nowTimestamp()
	updated, saveErr := upsertOrchestratorExecution(execution)
	if saveErr != nil {
		markOrchestratorExecutionFailed(execution, saveErr, nil)
		return
	}

	results, runErr := runOrchestratorTasks(ctx, updated, leases)
	if runErr != nil {
		markOrchestratorExecutionFailed(updated, runErr, results)
		if _, err := reclaimExecutionLeases(context.Background(), updated.ID, true); err != nil {
			logOrchestratorPersistError("reclaim execution leases on failure", err)
		}
		return
	}

	if _, err := reclaimExecutionLeases(context.Background(), updated.ID, true); err != nil {
		logOrchestratorPersistError("reclaim execution leases on success", err)
	}
	updated.Results = results
	updated.Status = OrchestratorExecutionStatusCompleted
	updated.CompletedAt = nowTimestamp()
	updated.UpdatedAt = updated.CompletedAt
	updated.Error = ""
	if _, err := upsertOrchestratorExecution(updated); err != nil {
		logOrchestratorPersistError("upsert orchestrator execution completed", err)
	}
}

func markOrchestratorExecutionFailed(execution OrchestratorExecution, runErr error, results []OrchestratorTaskResult) {
	execution.Status = OrchestratorExecutionStatusFailed
	execution.Error = strings.TrimSpace(runErr.Error())
	execution.UpdatedAt = nowTimestamp()
	execution.CompletedAt = execution.UpdatedAt
	if results != nil {
		execution.Results = results
	}
	if _, err := upsertOrchestratorExecution(execution); err != nil {
		logOrchestratorPersistError("upsert orchestrator execution failed", err)
	}
}

func provisionOrchestratorWorkers(ctx context.Context, execution OrchestratorExecution) ([]OrchestratorWorkerLease, error) {
	leases := make([]OrchestratorWorkerLease, 0)
	installedByRun := map[string]bool{}

	for _, req := range execution.RequiredWorkers {
		host, found, err := getRemoteHost(req.HostID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("remote host %s not found", req.HostID)
		}
		key := strings.ToLower(strings.TrimSpace(req.HostID) + ":" + strings.TrimSpace(req.AgentID))
		for i := 0; i < req.Count; i++ {
			lease := OrchestratorWorkerLease{
				ID:          uuid.NewString(),
				ExecutionID: execution.ID,
				HostID:      req.HostID,
				AgentID:     req.AgentID,
				State:       OrchestratorWorkerStateProvisioning,
				CreatedAt:   nowTimestamp(),
				UpdatedAt:   nowTimestamp(),
				HeartbeatAt: nowTimestamp(),
				LeaseExpireAt: time.Now().UTC().Add(defaultOrchestratorWorkerIdleTTL).
					Format(time.RFC3339Nano),
			}
			if _, saveErr := upsertOrchestratorWorkerLease(lease); saveErr != nil {
				return nil, saveErr
			}

			exists, existsErr := remoteAgentConfigExists(ctx, host, req.AgentID)
			if existsErr != nil {
				lease.State = OrchestratorWorkerStateError
				lease.LastError = existsErr.Error()
				lease.UpdatedAt = nowTimestamp()
				if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
					logOrchestratorPersistError("upsert orchestrator worker lease config-check error", err)
				}
				return nil, existsErr
			}

			ephemeral := !exists || installedByRun[key]
			if !exists && !installedByRun[key] {
				installResult, installErr := orchestratorInstallAgent(ctx, host, req.HostID, req.AgentID, false)
				if installErr != nil {
					lease.State = OrchestratorWorkerStateError
					lease.LastError = installErr.Error()
					lease.UpdatedAt = nowTimestamp()
					if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
						logOrchestratorPersistError("upsert orchestrator worker lease install error", err)
					}
					return nil, installErr
				}
				if installResult == nil || !installResult.Installed {
					return nil, fmt.Errorf("worker install did not complete for %s:%s", req.HostID, req.AgentID)
				}
				installedByRun[key] = true
				ephemeral = true
			}

			lease.Ephemeral = ephemeral
			lease.InstalledByRun = installedByRun[key]
			lease.State = OrchestratorWorkerStateReady
			lease.LastError = ""
			lease.HeartbeatAt = nowTimestamp()
			lease.UpdatedAt = nowTimestamp()
			savedLease, saveErr := upsertOrchestratorWorkerLease(lease)
			if saveErr != nil {
				return nil, saveErr
			}
			leases = append(leases, savedLease)
		}
	}
	return leases, nil
}

func runOrchestratorTasks(ctx context.Context, execution OrchestratorExecution, leases []OrchestratorWorkerLease) ([]OrchestratorTaskResult, error) {
	if len(execution.TaskUnits) == 0 {
		return []OrchestratorTaskResult{}, nil
	}

	pools := map[string]orchestratorWorkerPool{}
	firstPoolKey := ""
	for _, lease := range leases {
		key := workerPoolKey(lease.HostID, lease.AgentID)
		pool, ok := pools[key]
		if !ok {
			pool = orchestratorWorkerPool{
				key: key,
				ch:  make(chan OrchestratorWorkerLease, len(leases)),
			}
			pools[key] = pool
			if firstPoolKey == "" {
				firstPoolKey = key
			}
		}
		pool.ch <- lease
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("no available workers")
	}

	maxConcurrency := execution.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if maxConcurrency > len(execution.TaskUnits) {
		maxConcurrency = len(execution.TaskUnits)
	}
	type taskOutcome struct {
		index  int
		result OrchestratorTaskResult
		err    error
	}
	jobs := make(chan int)
	outcomes := make(chan taskOutcome, len(execution.TaskUnits))
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				task := execution.TaskUnits[idx]
				result, err := runTaskWithRetries(ctx, execution, task, pools, firstPoolKey)
				outcomes <- taskOutcome{
					index:  idx,
					result: result,
					err:    err,
				}
			}
		}()
	}

	for i := range execution.TaskUnits {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(outcomes)

	results := make([]OrchestratorTaskResult, len(execution.TaskUnits))
	var firstErr error
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		if outcome.err != nil && firstErr == nil {
			firstErr = outcome.err
		}
	}
	if firstErr != nil {
		return results, firstErr
	}
	return results, nil
}

func runTaskWithRetries(
	ctx context.Context,
	execution OrchestratorExecution,
	task OrchestratorTaskUnit,
	pools map[string]orchestratorWorkerPool,
	firstPoolKey string,
) (OrchestratorTaskResult, error) {
	retries := task.RetryBudget
	var lastErr error
	var lastResult OrchestratorTaskResult
	for attempt := 1; attempt <= retries+1; attempt++ {
		lease, key, err := acquireWorkerForTask(ctx, task, pools, firstPoolKey)
		if err != nil {
			lastErr = err
			lastResult = OrchestratorTaskResult{
				TaskID:   task.ID,
				Status:   OrchestratorTaskStatusFailed,
				Attempts: attempt,
				Error:    err.Error(),
			}
			continue
		}

		lease.State = OrchestratorWorkerStateBusy
		lease.HeartbeatAt = nowTimestamp()
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			logOrchestratorPersistError("upsert orchestrator worker lease busy", err)
		}

		start := time.Now()
		result, execErr := runOrchestratorTaskAttempt(ctx, execution, task, lease, attempt)
		lease.HeartbeatAt = nowTimestamp()
		lease.UpdatedAt = nowTimestamp()
		if execErr != nil {
			lease.State = OrchestratorWorkerStateReady
			lease.LastError = execErr.Error()
			if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
				logOrchestratorPersistError("upsert orchestrator worker lease ready-on-error", err)
			}
			releaseWorkerToPool(lease, pools[key])

			lastErr = execErr
			lastResult = result
			lastResult.LatencyMs = time.Since(start).Milliseconds()
			if attempt > retries {
				return lastResult, execErr
			}
			continue
		}

		lease.State = OrchestratorWorkerStateReady
		lease.LastError = ""
		lease.TaskCount++
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			logOrchestratorPersistError("upsert orchestrator worker lease ready", err)
		}
		releaseWorkerToPool(lease, pools[key])

		result.Status = OrchestratorTaskStatusCompleted
		result.Attempts = attempt
		result.LatencyMs = time.Since(start).Milliseconds()
		return result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("task failed with unknown error")
	}
	return lastResult, lastErr
}

func runOrchestratorTaskAttempt(
	ctx context.Context,
	execution OrchestratorExecution,
	task OrchestratorTaskUnit,
	lease OrchestratorWorkerLease,
	attempt int,
) (OrchestratorTaskResult, error) {
	host, found, err := getRemoteHost(lease.HostID)
	if err != nil {
		return OrchestratorTaskResult{
			TaskID:   task.ID,
			Status:   OrchestratorTaskStatusFailed,
			WorkerID: lease.ID,
			HostID:   lease.HostID,
			AgentID:  lease.AgentID,
			Attempts: attempt,
			Error:    err.Error(),
		}, err
	}
	if !found {
		runErr := fmt.Errorf("remote host %s not found", lease.HostID)
		return OrchestratorTaskResult{
			TaskID:   task.ID,
			Status:   OrchestratorTaskStatusFailed,
			WorkerID: lease.ID,
			HostID:   lease.HostID,
			AgentID:  lease.AgentID,
			Attempts: attempt,
			Error:    runErr.Error(),
		}, runErr
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sessionID := strings.TrimSpace(task.SessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("%s-%s-%d", execution.ID, task.ID, attempt)
	}

	start := time.Now()
	runResult, _, runErr := remoteRunTaskViaAgent(runCtx, host, lease.HostID, lease.AgentID, task.Input, sessionID)
	if runErr != nil {
		return OrchestratorTaskResult{
			TaskID:      task.ID,
			Status:      OrchestratorTaskStatusFailed,
			WorkerID:    lease.ID,
			HostID:      lease.HostID,
			AgentID:     lease.AgentID,
			Attempts:    attempt,
			Error:       runErr.Error(),
			StartedAt:   start.UTC().Format(time.RFC3339Nano),
			CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
			LatencyMs:   time.Since(start).Milliseconds(),
		}, runErr
	}
	return OrchestratorTaskResult{
		TaskID:      task.ID,
		Status:      OrchestratorTaskStatusCompleted,
		WorkerID:    lease.ID,
		HostID:      lease.HostID,
		AgentID:     lease.AgentID,
		Attempts:    attempt,
		Output:      strings.TrimSpace(runResult.Output),
		StartedAt:   start.UTC().Format(time.RFC3339Nano),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
		LatencyMs:   time.Since(start).Milliseconds(),
	}, nil
}

func workerPoolKey(hostID, agentID string) string {
	return strings.ToLower(strings.TrimSpace(hostID) + ":" + strings.TrimSpace(agentID))
}

func acquireWorkerForTask(
	ctx context.Context,
	task OrchestratorTaskUnit,
	pools map[string]orchestratorWorkerPool,
	firstPoolKey string,
) (OrchestratorWorkerLease, string, error) {
	hostID := strings.TrimSpace(task.HostID)
	agentID := strings.TrimSpace(task.AgentID)
	key := ""
	if hostID != "" {
		if agentID == "" {
			agentID = "zeroclaw"
		}
		key = workerPoolKey(hostID, agentID)
	} else {
		key = firstPoolKey
	}
	pool, ok := pools[key]
	if !ok {
		return OrchestratorWorkerLease{}, "", fmt.Errorf("no worker pool available for key %s", key)
	}
	select {
	case <-ctx.Done():
		return OrchestratorWorkerLease{}, "", ctx.Err()
	case lease := <-pool.ch:
		return lease, key, nil
	}
}

func releaseWorkerToPool(lease OrchestratorWorkerLease, pool orchestratorWorkerPool) {
	if lease.State == OrchestratorWorkerStateReclaimed {
		return
	}
	select {
	case pool.ch <- lease:
	default:
	}
}

func reclaimExecutionLeases(ctx context.Context, executionID string, force bool) (map[string]interface{}, error) {
	leases, err := orchestratorListLeasesByExecution(executionID)
	if err != nil {
		return nil, err
	}
	return reclaimOrchestratorLeaseSet(ctx, leases, force, 0)
}

func reclaimOrchestratorWorkers(ctx context.Context, force bool, idleTTL time.Duration) (map[string]interface{}, error) {
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		return nil, err
	}
	return reclaimOrchestratorLeaseSet(ctx, leases, force, idleTTL)
}

func reclaimOrchestratorLeaseSet(ctx context.Context, leases []OrchestratorWorkerLease, force bool, idleTTL time.Duration) (map[string]interface{}, error) {
	now := time.Now().UTC()
	reclaimed := 0
	skipped := 0
	failed := 0
	failures := make([]string, 0)

	recordPersistFailure := func(leaseID, step string, err error) {
		if err == nil {
			return
		}
		failed++
		msg := fmt.Sprintf("%s: persist %s failed: %v", strings.TrimSpace(leaseID), strings.TrimSpace(step), err)
		failures = append(failures, msg)
		logOrchestratorPersistError("upsert orchestrator worker lease "+strings.TrimSpace(step), err)
	}

	for _, lease := range leases {
		if lease.State == OrchestratorWorkerStateReclaimed {
			skipped++
			continue
		}
		if !force && idleTTL > 0 {
			heartbeat := parseRFC3339OrNow(lease.HeartbeatAt)
			if now.Sub(heartbeat) < idleTTL {
				skipped++
				continue
			}
		}
		if !lease.Ephemeral && !force {
			skipped++
			continue
		}

		lease.State = OrchestratorWorkerStateReclaiming
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			recordPersistFailure(lease.ID, "reclaiming", err)
		}

		if lease.Ephemeral {
			host, found, hostErr := getRemoteHost(lease.HostID)
			if hostErr != nil {
				failed++
				failures = append(failures, fmt.Sprintf("%s: %v", lease.ID, hostErr))
				lease.State = OrchestratorWorkerStateError
				lease.LastError = hostErr.Error()
				lease.UpdatedAt = nowTimestamp()
				if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
					recordPersistFailure(lease.ID, "host-error", err)
				}
				continue
			}
			if found {
				_, uninstallErr := remoteUninstallAgent(ctx, host, lease.HostID, lease.AgentID)
				if uninstallErr != nil {
					failed++
					failures = append(failures, fmt.Sprintf("%s: %v", lease.ID, uninstallErr))
					lease.State = OrchestratorWorkerStateError
					lease.LastError = uninstallErr.Error()
					lease.UpdatedAt = nowTimestamp()
					if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
						recordPersistFailure(lease.ID, "uninstall-error", err)
					}
					continue
				}
			}
		}

		lease.State = OrchestratorWorkerStateReclaimed
		lease.LastError = ""
		lease.HeartbeatAt = nowTimestamp()
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			recordPersistFailure(lease.ID, "reclaimed", err)
		}
		reclaimed++
	}

	summary := map[string]interface{}{
		"reclaimed": reclaimed,
		"skipped":   skipped,
		"failed":    failed,
		"failures":  failures,
	}
	if failed > 0 {
		return summary, fmt.Errorf("%d worker reclaim operations failed", failed)
	}
	return summary, nil
}

func parseRFC3339OrNow(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Now().UTC()
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}

func remoteAgentConfigExists(ctx context.Context, host RemoteHost, agentID string) (bool, error) {
	path := remoteOpenClawConfigPath
	switch normalizeRemoteInstallAgentID(agentID) {
	case "picoclaw":
		path = remotePicoClawConfigPath
	case "zeroclaw":
		path = remoteZeroClawConfigPath
	}
	exists, _, err := remoteConfigFileExists(ctx, host, path)
	return exists, err
}

func remoteRunTaskViaAgent(ctx context.Context, host RemoteHost, hostID, agentID, message, sessionID string) (*remoteRunResult, []remoteExecResult, error) {
	switch normalizeRemoteInstallAgentID(agentID) {
	case "zeroclaw":
		return remoteRunViaZeroClaw(ctx, host, hostID, agentID, message, sessionID)
	default:
		return remoteRunViaOpenClaw(ctx, host, hostID, agentID, message, sessionID)
	}
}

func remoteRunViaZeroClaw(ctx context.Context, host RemoteHost, hostID, agentID, message, sessionID string) (*remoteRunResult, []remoteExecResult, error) {
	startedAt := time.Now()
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return nil, nil, fmt.Errorf("message is required")
	}

	contract := buildRemoteMemoryRuntimeContract(agentID)
	candidates := []string{
		fmt.Sprintf("zeroclaw task run --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
		fmt.Sprintf("zeroclaw run --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
		fmt.Sprintf("zeroclaw agent --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
	}
	if strings.TrimSpace(sessionID) != "" {
		for i := range candidates {
			candidates[i] += " --session-id " + shellSingleQuote(strings.TrimSpace(sessionID))
		}
	}

	steps := make([]remoteExecResult, 0, len(candidates)+1)
	for _, cmd := range candidates {
		res, err := runRemoteCommand(ctx, host, wrapRemoteCommandWithMemoryContract(cmd, contract))
		steps = append(steps, res)
		if err != nil {
			continue
		}
		if res.ExitCode != 0 {
			continue
		}
		payload := parseRemoteRunPayload(res.Stdout, contract)
		return &remoteRunResult{
			HostID:     strings.TrimSpace(hostID),
			AgentID:    strings.TrimSpace(agentID),
			SessionID:  strings.TrimSpace(anyToString(payload["sessionId"])),
			Output:     extractChatResponseText(payload),
			LatencyMs:  time.Since(startedAt).Milliseconds(),
			Memory:     parseRemoteMemoryStatus(payload),
			RawPayload: payload,
		}, steps, nil
	}

	fallback, fallbackSteps, fallbackErr := remoteRunViaOpenClaw(ctx, host, hostID, agentID, message, sessionID)
	steps = append(steps, fallbackSteps...)
	if fallbackErr == nil {
		return fallback, steps, nil
	}
	return nil, steps, fmt.Errorf("zeroclaw direct run failed and openclaw fallback failed: %w", fallbackErr)
}

func parseRemoteRunPayload(stdout string, contract remoteMemoryRuntimeContract) map[string]interface{} {
	payload := strings.TrimSpace(extractJSONObjectOrArray(stdout))
	if payload == "" {
		return defaultRemoteRunPayload(stdout, contract)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return defaultRemoteRunPayload(stdout, contract)
	}
	if _, ok := out["memory"]; !ok {
		out["memory"] = defaultRemoteRunMemoryPayload(contract)
	}
	return out
}

func defaultRemoteRunPayload(stdout string, contract remoteMemoryRuntimeContract) map[string]interface{} {
	return map[string]interface{}{
		"message": strings.TrimSpace(stdout),
		"memory":  defaultRemoteRunMemoryPayload(contract),
	}
}

func defaultRemoteRunMemoryPayload(contract remoteMemoryRuntimeContract) map[string]interface{} {
	return map[string]interface{}{
		"contractId":     contract.ContractID,
		"contractDigest": contract.ContractDigest,
		"syncState":      "ready",
		"syncedAt":       nowTimestamp(),
	}
}
