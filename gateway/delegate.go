package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	delegateExecutionStatusQueued    = "queued"
	delegateExecutionStatusRunning   = "running"
	delegateExecutionStatusCompleted = "completed"
	delegateExecutionStatusFailed    = "failed"

	delegateTaskStatusCompleted = "completed"
	delegateTaskStatusFailed    = "failed"

	defaultDelegateTaskTimeout = 60 * time.Second
	maxDelegateConcurrency     = 8
)

type delegateTaskSpec struct {
	ID      string `json:"id"`
	Input   string `json:"input"`
	AgentID string `json:"agentId,omitempty"`
}

type delegateTaskResult struct {
	TaskID      string `json:"taskId"`
	Status      string `json:"status"`
	WorkerScope string `json:"workerScope"`
	HostID      string `json:"hostId,omitempty"`
	AgentID     string `json:"agentId,omitempty"`
	Attempts    int    `json:"attempts"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	LatencyMs   int64  `json:"latencyMs,omitempty"`
}

type delegateExecution struct {
	ID             string               `json:"id"`
	Goal           string               `json:"goal"`
	Provider       string               `json:"provider,omitempty"`
	ChatID         string               `json:"chatId,omitempty"`
	Actor          string               `json:"actor,omitempty"`
	Status         string               `json:"status"`
	PlannerWarning string               `json:"plannerWarning,omitempty"`
	TaskUnits      []delegateTaskSpec   `json:"taskUnits"`
	Results        []delegateTaskResult `json:"results,omitempty"`
	Error          string               `json:"error,omitempty"`
	CreatedAt      string               `json:"createdAt"`
	StartedAt      string               `json:"startedAt,omitempty"`
	CompletedAt    string               `json:"completedAt,omitempty"`
	UpdatedAt      string               `json:"updatedAt"`
}

type delegateStoreState struct {
	Executions []delegateExecution `json:"executions,omitempty"`
}

type delegateWorker struct {
	Scope   string
	HostID  string
	AgentID string
}

type delegateEvent struct {
	ExecutionID string `json:"executionId"`
	Status      string `json:"status"`
	Goal        string `json:"goal"`
	Error       string `json:"error,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type delegateWorkerScheduler struct {
	mu            sync.Mutex
	cond          *sync.Cond
	workers       []delegateWorker
	busy          []bool
	agentPresence map[string]struct{}
}

var (
	delegateStoreMu  sync.Mutex
	delegateRunState = struct {
		mu      sync.Mutex
		running map[string]bool
	}{
		running: map[string]bool{},
	}
	delegateEventSubscribers = struct {
		mu     sync.Mutex
		nextID int
		subs   map[int]chan delegateEvent
	}{
		subs: map[int]chan delegateEvent{},
	}
	delegateStartExecutionFn        = startDelegateExecutionAsync
	delegateDiscoverLocalWorkersFn  = discoverDelegateLocalWorkers
	delegateDiscoverRemoteWorkersFn = discoverDelegateRemoteWorkers
)

func handleDelegate(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/delegate <goal...> | /delegate status <execution_id>")
	}
	if strings.EqualFold(strings.TrimSpace(cmd.Args[0]), "status") {
		if len(cmd.Args) < 2 {
			return usageResp(cmd.RequestID, "/delegate status <execution_id>")
		}
		executionID := strings.TrimSpace(cmd.Args[1])
		if executionID == "" {
			return usageResp(cmd.RequestID, "/delegate status <execution_id>")
		}
		execution, found, err := getDelegateExecution(executionID)
		if err != nil {
			return errResp(cmd.RequestID, "E_INTERNAL", "failed to load delegate execution status")
		}
		if !found {
			return errResp(cmd.RequestID, "E_NOT_FOUND", fmt.Sprintf("delegate execution %s not found", executionID))
		}
		return GatewayResponse{
			RequestID: cmd.RequestID,
			Result:    "ok",
			Message:   renderDelegateExecutionStatus(execution),
		}
	}

	if daemon == nil {
		return errResp(cmd.RequestID, "E_INTERNAL", "daemon client is not configured")
	}

	goal := strings.TrimSpace(strings.Join(cmd.Args, " "))
	if goal == "" {
		return usageResp(cmd.RequestID, "/delegate <goal...>")
	}

	execution, err := submitDelegateExecution(ctx, daemon, cmd.Provider, cmd.ChatID, actor, cmd.RequestID, goal)
	if err != nil {
		return errResp(cmd.RequestID, "E_COMMAND_FAILED", err.Error())
	}

	lines := []string{
		fmt.Sprintf("delegate execution accepted: %s", execution.ID),
		fmt.Sprintf("Use `/delegate status %s` to check progress.", execution.ID),
	}
	if strings.TrimSpace(execution.PlannerWarning) != "" {
		lines = append(lines, "Planner warning: "+execution.PlannerWarning)
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   strings.Join(lines, "\n"),
	}
}

func submitDelegateExecution(
	ctx context.Context,
	daemon *DaemonClient,
	provider string,
	chatID string,
	actor string,
	requestID string,
	goal string,
) (delegateExecution, error) {
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		return delegateExecution{}, errors.New("goal is required")
	}
	tasks, plannerWarning := decomposeDelegateGoal(ctx, daemon, trimmedGoal, actor, requestID)
	if len(tasks) == 0 {
		tasks = []delegateTaskSpec{{ID: "task-1", Input: trimmedGoal}}
		if strings.TrimSpace(plannerWarning) == "" {
			plannerWarning = "planner returned no tasks; fell back to a single task"
		}
	}

	now := nowTimestamp()
	execution := delegateExecution{
		ID:             uuid.NewString(),
		Goal:           trimmedGoal,
		Provider:       strings.TrimSpace(provider),
		ChatID:         strings.TrimSpace(chatID),
		Actor:          strings.TrimSpace(actor),
		Status:         delegateExecutionStatusQueued,
		PlannerWarning: strings.TrimSpace(plannerWarning),
		TaskUnits:      tasks,
		Results:        []delegateTaskResult{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := upsertDelegateExecution(execution); err != nil {
		return delegateExecution{}, err
	}
	delegateStartExecutionFn(execution.ID, daemon)
	return execution, nil
}

func decomposeDelegateGoal(
	ctx context.Context,
	daemon *DaemonClient,
	goal string,
	actor string,
	requestID string,
) ([]delegateTaskSpec, string) {
	if daemon == nil {
		return nil, "daemon client is not configured; using single-task fallback"
	}
	tasks, err := daemon.DecomposeBaseAgent(ctx, strings.TrimSpace(goal), actor, requestID)
	if err != nil {
		return []delegateTaskSpec{{ID: "task-1", Input: strings.TrimSpace(goal)}}, err.Error()
	}
	normalized := make([]delegateTaskSpec, 0, len(tasks))
	seen := map[string]struct{}{}
	for i, task := range tasks {
		input := strings.TrimSpace(task.Input)
		if input == "" {
			continue
		}
		id := strings.TrimSpace(task.ID)
		if id == "" {
			id = fmt.Sprintf("task-%d", i+1)
		}
		if _, ok := seen[id]; ok {
			id = fmt.Sprintf("%s-%d", id, i+1)
		}
		seen[id] = struct{}{}
		agentID := strings.ToLower(strings.TrimSpace(task.AgentID))
		if !isDelegateSupportedAgent(agentID) {
			agentID = ""
		}
		normalized = append(normalized, delegateTaskSpec{
			ID:      id,
			Input:   input,
			AgentID: agentID,
		})
	}
	if len(normalized) == 0 {
		return []delegateTaskSpec{{ID: "task-1", Input: strings.TrimSpace(goal)}}, "planner returned empty task list; using single-task fallback"
	}
	return normalized, ""
}

func startDelegateExecutionAsync(executionID string, daemon *DaemonClient) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return
	}
	delegateRunState.mu.Lock()
	if delegateRunState.running[id] {
		delegateRunState.mu.Unlock()
		return
	}
	delegateRunState.running[id] = true
	delegateRunState.mu.Unlock()

	go func() {
		defer func() {
			delegateRunState.mu.Lock()
			delete(delegateRunState.running, id)
			delegateRunState.mu.Unlock()
		}()
		runDelegateExecution(id, daemon)
	}()
}

func runDelegateExecution(executionID string, daemon *DaemonClient) {
	execution, found, err := getDelegateExecution(executionID)
	if err != nil || !found {
		return
	}
	if execution.Status == delegateExecutionStatusCompleted || execution.Status == delegateExecutionStatusFailed {
		return
	}

	execution.Status = delegateExecutionStatusRunning
	execution.StartedAt = nowTimestamp()
	execution.UpdatedAt = execution.StartedAt
	execution.Error = ""
	if _, err := upsertDelegateExecution(execution); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	workers, workerErr := collectDelegateWorkers(ctx, daemon, execution.ID)
	if workerErr != nil {
		execution.Status = delegateExecutionStatusFailed
		execution.Error = workerErr.Error()
		execution.CompletedAt = nowTimestamp()
		execution.UpdatedAt = execution.CompletedAt
		_, _ = upsertDelegateExecution(execution)
		publishDelegateEvent(execution)
		return
	}

	results, runErr := runDelegateTasks(ctx, daemon, execution, workers)
	execution.Results = results
	execution.CompletedAt = nowTimestamp()
	execution.UpdatedAt = execution.CompletedAt
	if runErr != nil {
		execution.Status = delegateExecutionStatusFailed
		execution.Error = runErr.Error()
	} else {
		execution.Status = delegateExecutionStatusCompleted
		execution.Error = ""
	}
	_, _ = upsertDelegateExecution(execution)
	publishDelegateEvent(execution)
}

func collectDelegateWorkers(ctx context.Context, daemon *DaemonClient, requestID string) ([]delegateWorker, error) {
	localWorkers, localErr := delegateDiscoverLocalWorkersFn(ctx, daemon, requestID)
	remoteWorkers, remoteErr := delegateDiscoverRemoteWorkersFn(ctx, requestID)

	workers := make([]delegateWorker, 0, len(localWorkers)+len(remoteWorkers))
	workers = append(workers, localWorkers...)
	workers = append(workers, remoteWorkers...)
	if len(workers) > 0 {
		return workers, nil
	}

	errs := make([]string, 0, 2)
	if localErr != nil {
		errs = append(errs, "local discovery failed: "+localErr.Error())
	}
	if remoteErr != nil {
		errs = append(errs, "remote discovery failed: "+remoteErr.Error())
	}
	if len(errs) == 0 {
		errs = append(errs, "no available picoclaw/zeroclaw workers")
	}
	return nil, errors.New(strings.Join(errs, "; "))
}

func discoverDelegateLocalWorkers(ctx context.Context, daemon *DaemonClient, requestID string) ([]delegateWorker, error) {
	if daemon == nil {
		return nil, errors.New("daemon client is not configured")
	}
	agents, err := daemon.ListAgents(ctx, "gateway:delegate:discover-local", requestID)
	if err != nil {
		return nil, err
	}
	workers := make([]delegateWorker, 0, len(agents))
	for _, agent := range agents {
		agentID := strings.ToLower(strings.TrimSpace(agent.ID))
		if !isDelegateSupportedAgent(agentID) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(agent.InstallState), "installed") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(agent.Runtime), "running") {
			continue
		}
		workers = append(workers, delegateWorker{
			Scope:   "local",
			AgentID: agentID,
		})
	}
	return workers, nil
}

func discoverDelegateRemoteWorkers(ctx context.Context, requestID string) ([]delegateWorker, error) {
	hosts, err := listRemoteHosts()
	if err != nil {
		return nil, err
	}
	workers := make([]delegateWorker, 0)
	seen := map[string]struct{}{}
	for _, host := range hosts {
		hostCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		instances, _, listErr := remoteListInstancesForHost(hostCtx, host, host.ID)
		cancel()
		if listErr != nil {
			continue
		}
		for _, inst := range instances {
			agentID := strings.ToLower(strings.TrimSpace(inst.AgentID))
			if !isDelegateSupportedAgent(agentID) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(inst.RuntimeState), "running") {
				continue
			}
			key := workerPoolKey(host.ID, agentID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			workers = append(workers, delegateWorker{
				Scope:   "remote",
				HostID:  strings.TrimSpace(host.ID),
				AgentID: agentID,
			})
		}
	}
	sort.Slice(workers, func(i, j int) bool {
		if workers[i].HostID == workers[j].HostID {
			return workers[i].AgentID < workers[j].AgentID
		}
		return workers[i].HostID < workers[j].HostID
	})
	return workers, nil
}

func runDelegateTasks(
	ctx context.Context,
	daemon *DaemonClient,
	execution delegateExecution,
	workers []delegateWorker,
) ([]delegateTaskResult, error) {
	if len(execution.TaskUnits) == 0 {
		return []delegateTaskResult{}, nil
	}
	if len(workers) == 0 {
		return nil, errors.New("no workers are available")
	}

	workerScheduler := newDelegateWorkerScheduler(workers)

	taskCount := len(execution.TaskUnits)
	workerCount := len(workers)
	if workerCount > maxDelegateConcurrency {
		workerCount = maxDelegateConcurrency
	}
	if workerCount > taskCount {
		workerCount = taskCount
	}

	type taskOutcome struct {
		index  int
		result delegateTaskResult
		err    error
	}

	jobs := make(chan int)
	outcomes := make(chan taskOutcome, taskCount)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				task := execution.TaskUnits[idx]
				workerIdx, worker := workerScheduler.acquire(task.AgentID)
				result, runErr := runDelegateTask(ctx, daemon, execution.ID, task, worker, idx)
				workerScheduler.release(workerIdx)
				outcomes <- taskOutcome{
					index:  idx,
					result: result,
					err:    runErr,
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

	results := make([]delegateTaskResult, len(execution.TaskUnits))
	var firstErr error
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		if outcome.err != nil && firstErr == nil {
			firstErr = outcome.err
		}
	}
	return results, firstErr
}

func newDelegateWorkerScheduler(workers []delegateWorker) *delegateWorkerScheduler {
	copied := make([]delegateWorker, 0, len(workers))
	presence := make(map[string]struct{}, len(workers))
	for _, worker := range workers {
		copied = append(copied, worker)
		agentID := strings.ToLower(strings.TrimSpace(worker.AgentID))
		if agentID == "" {
			continue
		}
		presence[agentID] = struct{}{}
	}
	scheduler := &delegateWorkerScheduler{
		workers:       copied,
		busy:          make([]bool, len(copied)),
		agentPresence: presence,
	}
	scheduler.cond = sync.NewCond(&scheduler.mu)
	return scheduler
}

func (s *delegateWorkerScheduler) acquire(preferredAgentID string) (int, delegateWorker) {
	normalizedPreferred := strings.ToLower(strings.TrimSpace(preferredAgentID))
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if normalizedPreferred != "" {
			if _, ok := s.agentPresence[normalizedPreferred]; ok {
				if idx := s.findAvailableLocked(normalizedPreferred); idx >= 0 {
					s.busy[idx] = true
					return idx, s.workers[idx]
				}
			} else {
				if idx := s.findAvailableLocked(""); idx >= 0 {
					s.busy[idx] = true
					return idx, s.workers[idx]
				}
			}
		} else {
			if idx := s.findAvailableLocked(""); idx >= 0 {
				s.busy[idx] = true
				return idx, s.workers[idx]
			}
		}
		s.cond.Wait()
	}
}

func (s *delegateWorkerScheduler) release(index int) {
	s.mu.Lock()
	if index >= 0 && index < len(s.busy) {
		s.busy[index] = false
	}
	s.mu.Unlock()
	s.cond.Broadcast()
}

func (s *delegateWorkerScheduler) findAvailableLocked(agentID string) int {
	normalized := strings.ToLower(strings.TrimSpace(agentID))
	for idx, worker := range s.workers {
		if s.busy[idx] {
			continue
		}
		if normalized != "" && !strings.EqualFold(strings.TrimSpace(worker.AgentID), normalized) {
			continue
		}
		return idx
	}
	return -1
}

func runDelegateTask(
	ctx context.Context,
	daemon *DaemonClient,
	executionID string,
	task delegateTaskSpec,
	worker delegateWorker,
	index int,
) (delegateTaskResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, defaultDelegateTaskTimeout)
	defer cancel()

	start := time.Now()
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", index+1)
	}
	result := delegateTaskResult{
		TaskID:      taskID,
		Status:      delegateTaskStatusFailed,
		WorkerScope: strings.TrimSpace(worker.Scope),
		HostID:      strings.TrimSpace(worker.HostID),
		AgentID:     strings.TrimSpace(worker.AgentID),
		Attempts:    1,
		StartedAt:   start.UTC().Format(time.RFC3339Nano),
	}

	sessionID := fmt.Sprintf("delegate-%s-%s", strings.TrimSpace(executionID), taskID)
	requestID := fmt.Sprintf("delegate-%s", strings.TrimSpace(executionID))
	switch strings.ToLower(strings.TrimSpace(worker.Scope)) {
	case "local":
		if daemon == nil {
			result.Error = "daemon client is not configured"
			break
		}
		chatResult, err := daemon.ChatAgent(
			runCtx,
			worker.AgentID,
			task.Input,
			sessionID,
			"gateway:delegate:local",
			requestID,
		)
		if err != nil {
			result.Error = err.Error()
			break
		}
		result.Status = delegateTaskStatusCompleted
		result.Output = strings.TrimSpace(chatResult.Message)
	case "remote":
		host, found, err := getRemoteHost(worker.HostID)
		if err != nil {
			result.Error = err.Error()
			break
		}
		if !found {
			result.Error = fmt.Sprintf("remote host %s not found", worker.HostID)
			break
		}
		runResult, _, runErr := remoteRunTaskViaAgent(runCtx, host, worker.HostID, worker.AgentID, task.Input, sessionID)
		if runErr != nil {
			result.Error = runErr.Error()
			break
		}
		result.Status = delegateTaskStatusCompleted
		result.Output = strings.TrimSpace(runResult.Output)
	default:
		result.Error = fmt.Sprintf("unsupported worker scope %q", worker.Scope)
	}

	result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.LatencyMs = time.Since(start).Milliseconds()
	if strings.TrimSpace(result.Output) == "" && result.Status == delegateTaskStatusCompleted {
		result.Output = "(empty output)"
	}
	if strings.TrimSpace(result.Error) != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}

func isDelegateSupportedAgent(agentID string) bool {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw", "zeroclaw":
		return true
	default:
		return false
	}
}

func renderDelegateExecutionStatus(execution delegateExecution) string {
	total := len(execution.TaskUnits)
	completed := 0
	failed := 0
	for _, result := range execution.Results {
		switch strings.ToLower(strings.TrimSpace(result.Status)) {
		case delegateTaskStatusCompleted:
			completed++
		case delegateTaskStatusFailed:
			failed++
		}
	}

	lines := []string{
		fmt.Sprintf("delegate execution %s", strings.TrimSpace(execution.ID)),
		fmt.Sprintf("status: %s", strings.TrimSpace(execution.Status)),
		fmt.Sprintf("goal: %s", strings.TrimSpace(execution.Goal)),
		fmt.Sprintf("tasks: total=%d completed=%d failed=%d", total, completed, failed),
	}
	if strings.TrimSpace(execution.Error) != "" {
		lines = append(lines, "error: "+strings.TrimSpace(execution.Error))
	}
	if len(execution.Results) > 0 {
		lines = append(lines, "task results:")
		for _, result := range execution.Results {
			scope := strings.TrimSpace(result.WorkerScope)
			if scope == "" {
				scope = "unknown"
			}
			target := strings.TrimSpace(result.AgentID)
			if strings.EqualFold(scope, "remote") && strings.TrimSpace(result.HostID) != "" {
				target = strings.TrimSpace(result.HostID) + "/" + target
			}
			if target == "" {
				target = "(unknown target)"
			}
			summary := strings.TrimSpace(result.Error)
			if summary == "" {
				summary = strings.TrimSpace(result.Output)
			}
			if summary == "" {
				summary = "(no output)"
			}
			lines = append(lines, fmt.Sprintf(
				"- %s [%s] target=%s latency=%dms %s",
				strings.TrimSpace(result.TaskID),
				strings.TrimSpace(result.Status),
				target,
				result.LatencyMs,
				truncateDelegateText(summary, 140),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func truncateDelegateText(input string, limit int) string {
	trimmed := strings.TrimSpace(input)
	if limit <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func handleWebUIDelegateEvents(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "streaming not supported"))
		return
	}

	subID, ch := subscribeDelegateEvents()
	defer unsubscribeDelegateEvents(subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	_ = writeSSEEvent(w, map[string]interface{}{
		"type":      "start",
		"requestId": requestID,
	})
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			_ = writeSSEEvent(w, map[string]interface{}{
				"type":        "delegate-finish",
				"executionId": evt.ExecutionID,
				"status":      evt.Status,
				"goal":        evt.Goal,
				"error":       evt.Error,
				"completedAt": evt.CompletedAt,
			})
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

func publishDelegateEvent(execution delegateExecution) {
	evt := delegateEvent{
		ExecutionID: strings.TrimSpace(execution.ID),
		Status:      strings.TrimSpace(execution.Status),
		Goal:        strings.TrimSpace(execution.Goal),
		Error:       strings.TrimSpace(execution.Error),
		CompletedAt: strings.TrimSpace(execution.CompletedAt),
	}
	delegateEventSubscribers.mu.Lock()
	defer delegateEventSubscribers.mu.Unlock()
	for _, ch := range delegateEventSubscribers.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

func subscribeDelegateEvents() (int, chan delegateEvent) {
	delegateEventSubscribers.mu.Lock()
	defer delegateEventSubscribers.mu.Unlock()
	delegateEventSubscribers.nextID++
	id := delegateEventSubscribers.nextID
	ch := make(chan delegateEvent, 16)
	delegateEventSubscribers.subs[id] = ch
	return id, ch
}

func unsubscribeDelegateEvents(id int) {
	delegateEventSubscribers.mu.Lock()
	defer delegateEventSubscribers.mu.Unlock()
	ch, ok := delegateEventSubscribers.subs[id]
	if !ok {
		return
	}
	delete(delegateEventSubscribers.subs, id)
	close(ch)
}

func delegateStorePath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_DELEGATE_STORE")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for delegate store: %w", err)
	}
	return filepath.Join(home, ".carrier", "delegate-executions.json"), nil
}

func loadDelegateState() (*delegateStoreState, string, error) {
	path, err := delegateStorePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &delegateStoreState{Executions: []delegateExecution{}}, path, nil
		}
		return nil, "", fmt.Errorf("read delegate store: %w", err)
	}
	var state delegateStoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, "", fmt.Errorf("parse delegate store: %w", err)
	}
	if state.Executions == nil {
		state.Executions = []delegateExecution{}
	}
	return &state, path, nil
}

func saveDelegateState(path string, state *delegateStoreState) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("delegate store path is empty")
	}
	if state == nil {
		return errors.New("delegate store state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create delegate store dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal delegate store: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write delegate store: %w", err)
	}
	return nil
}

func upsertDelegateExecution(execution delegateExecution) (delegateExecution, error) {
	delegateStoreMu.Lock()
	defer delegateStoreMu.Unlock()
	state, path, err := loadDelegateState()
	if err != nil {
		return delegateExecution{}, err
	}
	trimmedID := strings.TrimSpace(execution.ID)
	if trimmedID == "" {
		return delegateExecution{}, errors.New("execution id is required")
	}
	execution.ID = trimmedID
	index := -1
	for i := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(state.Executions[i].ID), trimmedID) {
			index = i
			break
		}
	}
	if index >= 0 {
		execution.CreatedAt = state.Executions[index].CreatedAt
		state.Executions[index] = execution
	} else {
		state.Executions = append(state.Executions, execution)
	}
	if err := saveDelegateState(path, state); err != nil {
		return delegateExecution{}, err
	}
	return execution, nil
}

func getDelegateExecution(executionID string) (delegateExecution, bool, error) {
	delegateStoreMu.Lock()
	defer delegateStoreMu.Unlock()
	state, _, err := loadDelegateState()
	if err != nil {
		return delegateExecution{}, false, err
	}
	id := strings.TrimSpace(executionID)
	for _, execution := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(execution.ID), id) {
			return execution, true, nil
		}
	}
	return delegateExecution{}, false, nil
}

func listDelegateExecutions(limit int) ([]delegateExecution, error) {
	delegateStoreMu.Lock()
	defer delegateStoreMu.Unlock()
	state, _, err := loadDelegateState()
	if err != nil {
		return nil, err
	}
	executions := append([]delegateExecution(nil), state.Executions...)
	sort.Slice(executions, func(i, j int) bool {
		ti := parseRFC3339OrNow(executions[i].UpdatedAt)
		tj := parseRFC3339OrNow(executions[j].UpdatedAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return executions[i].ID > executions[j].ID
	})
	if limit > 0 && len(executions) > limit {
		executions = executions[:limit]
	}
	return executions, nil
}
