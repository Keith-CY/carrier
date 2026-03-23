package baseagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SubagentJobStatus string

const (
	SubagentJobStatusQueued    SubagentJobStatus = "queued"
	SubagentJobStatusRunning   SubagentJobStatus = "running"
	SubagentJobStatusAwaiting  SubagentJobStatus = "awaiting_context"
	SubagentJobStatusDegraded  SubagentJobStatus = "degraded"
	SubagentJobStatusCompleted SubagentJobStatus = "completed"
	SubagentJobStatusCancelled SubagentJobStatus = "cancelled"
	SubagentJobStatusFailed    SubagentJobStatus = "failed"
)

type SubagentJob struct {
	JobID            string                      `json:"jobId"`
	Task             string                      `json:"task"`
	Status           SubagentJobStatus           `json:"status"`
	Summary          string                      `json:"summary"`
	Result           string                      `json:"result"`
	Error            string                      `json:"error"`
	Contract         *DelegationContract         `json:"contract,omitempty"`
	ContextRequests  []DelegationContextRequest  `json:"contextRequests,omitempty"`
	ContextResponses []DelegationContextResponse `json:"contextResponses,omitempty"`
	MissingContext   []string                    `json:"missingContext,omitempty"`
	Confidence       string                      `json:"confidence,omitempty"`
	Outcome          *DelegationOutcome          `json:"outcome,omitempty"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
}

type SubagentExecutor func(context.Context, SubagentRequest) (string, error)

type SubagentManager interface {
	SubagentSpawner
	Job(ctx context.Context, jobID string) (SubagentJob, error)
	RecentJobs(ctx context.Context, limit int) []SubagentJob
	ContextRequests(ctx context.Context, jobID string) ([]DelegationContextRequest, error)
	RespondContextRequest(ctx context.Context, jobID, requestID string, response DelegationContextResponse) (DelegationContextResponse, error)
	Cancel(ctx context.Context, jobID string) error
}

type InMemorySubagentManager struct {
	mu             sync.RWMutex
	next           int
	jobs           map[string]SubagentJob
	order          []string
	maxHistory     int
	cancels        map[string]context.CancelFunc
	contextWaiters map[string]chan DelegationContextResponse
	executor       SubagentExecutor
	storagePath    string
}

func NewInMemorySubagentManager(executor SubagentExecutor) *InMemorySubagentManager {
	return NewInMemorySubagentManagerWithStorage(executor, "")
}

func NewInMemorySubagentManagerWithStorage(executor SubagentExecutor, storagePath string) *InMemorySubagentManager {
	if executor == nil {
		executor = func(_ context.Context, req SubagentRequest) (string, error) {
			task := strings.TrimSpace(req.Task)
			if task == "" {
				task = "delegated task"
			}
			return "delegated task accepted: " + task, nil
		}
	}
	manager := &InMemorySubagentManager{
		jobs:           map[string]SubagentJob{},
		order:          nil,
		maxHistory:     32,
		cancels:        map[string]context.CancelFunc{},
		contextWaiters: map[string]chan DelegationContextResponse{},
		executor:       executor,
		storagePath:    strings.TrimSpace(storagePath),
	}
	manager.loadPersistedJobs()
	return manager
}

func (m *InMemorySubagentManager) Spawn(ctx context.Context, req SubagentRequest) (SubagentJobHandle, error) {
	if m == nil {
		return SubagentJobHandle{}, fmt.Errorf("subagent manager is unavailable")
	}
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return SubagentJobHandle{}, fmt.Errorf("task is required")
	}
	req.Task = task

	now := time.Now().UTC()

	m.mu.Lock()
	m.next++
	jobID := fmt.Sprintf("subagent-%d", m.next)
	contract := normalizeDelegationContract(req.Contract, task)
	if contract.ContractID == "" {
		contract.ContractID = jobID
	}
	req.Contract = contract
	job := SubagentJob{
		JobID:      jobID,
		Task:       task,
		Status:     SubagentJobStatusQueued,
		Summary:    task,
		Contract:   cloneDelegationContract(contract),
		Confidence: "medium",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m.jobs[jobID] = job
	m.order = append(m.order, jobID)
	m.persistLocked()
	m.mu.Unlock()

	execBase := context.Background()
	if ctx != nil {
		execBase = context.WithoutCancel(ctx)
	}
	execCtx, cancel := context.WithCancel(execBase)

	m.mu.Lock()
	m.cancels[jobID] = cancel
	m.mu.Unlock()
	go m.runJob(execCtx, jobID, req)

	return SubagentJobHandle{
		JobID:      job.JobID,
		Status:     string(job.Status),
		Summary:    job.Summary,
		ContractID: contract.ContractID,
	}, nil
}

func (m *InMemorySubagentManager) runJob(ctx context.Context, jobID string, req SubagentRequest) {
	if m == nil {
		return
	}

	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return
	}
	job.Status = SubagentJobStatusRunning
	job.UpdatedAt = time.Now().UTC()
	m.jobs[jobID] = job
	m.mu.Unlock()

	execCtx := withDelegationContextBroker(ctx, &subagentContextBroker{
		manager:    m,
		jobID:      jobID,
		contractID: strings.TrimSpace(job.Contract.ContractID),
	})
	result, err := m.executor(execCtx, req)

	m.mu.Lock()
	job, ok = m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return
	}
	job.UpdatedAt = time.Now().UTC()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			job.Status = SubagentJobStatusCancelled
			job.Error = "subagent job cancelled"
		} else {
			job.Status = SubagentJobStatusFailed
			job.Error = strings.TrimSpace(err.Error())
		}
	} else {
		job.Status = SubagentJobStatusCompleted
		job.Result = strings.TrimSpace(result)
		job.Outcome = &DelegationOutcome{
			Summary:        strings.TrimSpace(firstNonEmptyString(job.Summary, job.Task)),
			Result:         strings.TrimSpace(result),
			MissingContext: trimStringList(job.MissingContext),
			Degraded:       len(job.MissingContext) > 0,
			Confidence:     strings.TrimSpace(firstNonEmptyString(job.Confidence, "medium")),
		}
	}
	m.jobs[jobID] = job
	delete(m.cancels, jobID)
	m.pruneLocked()
	m.persistLocked()
	m.mu.Unlock()
}

func (m *InMemorySubagentManager) Job(_ context.Context, jobID string) (SubagentJob, error) {
	if m == nil {
		return SubagentJob{}, fmt.Errorf("subagent manager is unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return SubagentJob{}, fmt.Errorf("job_id is required")
	}

	m.mu.RLock()
	job, ok := m.jobs[jobID]
	m.mu.RUnlock()
	if !ok {
		return SubagentJob{}, fmt.Errorf("subagent job %s not found", jobID)
	}
	return cloneSubagentJob(job), nil
}

func (m *InMemorySubagentManager) RecentJobs(_ context.Context, limit int) []SubagentJob {
	if m == nil {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make([]SubagentJob, 0, min(limit, len(m.order)))
	for idx := len(m.order) - 1; idx >= 0 && len(jobs) < limit; idx-- {
		jobID := m.order[idx]
		job, ok := m.jobs[jobID]
		if !ok {
			continue
		}
		jobs = append(jobs, cloneSubagentJob(job))
	}
	return jobs
}

func (m *InMemorySubagentManager) ContextRequests(_ context.Context, jobID string) ([]DelegationContextRequest, error) {
	if m == nil {
		return nil, fmt.Errorf("subagent manager is unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	m.mu.RLock()
	job, ok := m.jobs[jobID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("subagent job %s not found", jobID)
	}
	return cloneDelegationContextRequests(job.ContextRequests), nil
}

func (m *InMemorySubagentManager) RespondContextRequest(_ context.Context, jobID, requestID string, response DelegationContextResponse) (DelegationContextResponse, error) {
	if m == nil {
		return DelegationContextResponse{}, fmt.Errorf("subagent manager is unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	requestID = strings.TrimSpace(requestID)
	if jobID == "" || requestID == "" {
		return DelegationContextResponse{}, fmt.Errorf("job_id and request_id are required")
	}
	normalized := normalizeDelegationContextResponse(response, requestID, time.Now().UTC())

	m.mu.Lock()
	if _, ok := m.jobs[jobID]; !ok {
		m.mu.Unlock()
		return DelegationContextResponse{}, fmt.Errorf("subagent job %s not found", jobID)
	}
	waiter, hasWaiter := m.contextWaiters[m.contextWaiterKey(jobID, requestID)]
	m.mu.Unlock()

	if hasWaiter {
		select {
		case waiter <- normalized:
		default:
		}
		return normalized, nil
	}

	m.finalizeContextResponse(jobID, requestID, normalized)
	return normalized, nil
}

func (m *InMemorySubagentManager) Cancel(_ context.Context, jobID string) error {
	if m == nil {
		return fmt.Errorf("subagent manager is unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}

	m.mu.RLock()
	job, ok := m.jobs[jobID]
	cancel := m.cancels[jobID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("subagent job %s not found", jobID)
	}
	if job.Status == SubagentJobStatusCompleted || job.Status == SubagentJobStatusFailed || job.Status == SubagentJobStatusCancelled {
		return nil
	}
	if cancel == nil {
		return fmt.Errorf("subagent job %s is not cancellable", jobID)
	}
	cancel()
	return nil
}

type subagentContextBroker struct {
	manager    *InMemorySubagentManager
	jobID      string
	contractID string
}

func (b *subagentContextBroker) Request(ctx context.Context, req DelegationContextRequest) (DelegationContextResponse, error) {
	if b == nil || b.manager == nil {
		return DelegationContextResponse{}, fmt.Errorf("subagent context broker is unavailable")
	}
	return b.manager.requestContext(ctx, b.jobID, b.contractID, req)
}

func (m *InMemorySubagentManager) requestContext(ctx context.Context, jobID, contractID string, req DelegationContextRequest) (DelegationContextResponse, error) {
	now := time.Now().UTC()
	normalized := normalizeDelegationContextRequest(req, contractID, now)
	if normalized.Question == "" {
		return DelegationContextResponse{}, fmt.Errorf("context request question is required")
	}

	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return DelegationContextResponse{}, fmt.Errorf("subagent job %s not found", jobID)
	}
	if normalized.RequestID == "" {
		normalized.RequestID = fmt.Sprintf("%s-ctx-%d", jobID, len(job.ContextRequests)+1)
	}
	job.ContextRequests = append(job.ContextRequests, normalized)
	job.Status = SubagentJobStatusAwaiting
	job.UpdatedAt = now
	m.jobs[jobID] = job
	waiterKey := m.contextWaiterKey(jobID, normalized.RequestID)
	waiter := make(chan DelegationContextResponse, 1)
	m.contextWaiters[waiterKey] = waiter
	m.persistLocked()
	m.mu.Unlock()

	select {
	case response := <-waiter:
		m.finalizeContextResponse(jobID, normalized.RequestID, response)
		return normalizeDelegationContextResponse(response, normalized.RequestID, time.Now().UTC()), nil
	case <-ctx.Done():
		status := DelegationContextStatusUnavailable
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = DelegationContextStatusTimedOut
		}
		response := normalizeDelegationContextResponse(DelegationContextResponse{
			Status:  status,
			Summary: strings.TrimSpace(ctx.Err().Error()),
		}, normalized.RequestID, time.Now().UTC())
		m.finalizeContextResponse(jobID, normalized.RequestID, response)
		if errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return response, ctx.Err()
		}
		return response, nil
	}
}

func (m *InMemorySubagentManager) finalizeContextResponse(jobID, requestID string, response DelegationContextResponse) {
	if m == nil {
		return
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		delete(m.contextWaiters, m.contextWaiterKey(jobID, requestID))
		return
	}
	delete(m.contextWaiters, m.contextWaiterKey(jobID, requestID))
	response = normalizeDelegationContextResponse(response, requestID, now)
	for idx := range job.ContextRequests {
		if strings.TrimSpace(job.ContextRequests[idx].RequestID) != requestID {
			continue
		}
		job.ContextRequests[idx].Status = response.Status
		break
	}
	replaced := false
	for idx := range job.ContextResponses {
		if strings.TrimSpace(job.ContextResponses[idx].RequestID) != requestID {
			continue
		}
		job.ContextResponses[idx] = response
		replaced = true
		break
	}
	if !replaced {
		job.ContextResponses = append(job.ContextResponses, response)
	}
	if response.Status != DelegationContextStatusFulfilled {
		missing := requestID
		for _, item := range job.ContextRequests {
			if strings.TrimSpace(item.RequestID) == requestID && strings.TrimSpace(item.Question) != "" {
				missing = strings.TrimSpace(item.Question)
				break
			}
		}
		job.MissingContext = trimStringList(append(job.MissingContext, missing))
		job.Status = SubagentJobStatusDegraded
		job.Confidence = "low"
	} else {
		if job.Status == SubagentJobStatusAwaiting {
			job.Status = SubagentJobStatusRunning
		}
		if strings.TrimSpace(job.Confidence) == "" {
			job.Confidence = "medium"
		}
	}
	job.UpdatedAt = now
	m.jobs[jobID] = job
	m.persistLocked()
}

func (m *InMemorySubagentManager) contextWaiterKey(jobID, requestID string) string {
	return strings.TrimSpace(jobID) + ":" + strings.TrimSpace(requestID)
}

func (m *InMemorySubagentManager) pruneLocked() {
	if m == nil || m.maxHistory <= 0 {
		return
	}
	for len(m.order) > m.maxHistory {
		oldestID := m.order[0]
		job, ok := m.jobs[oldestID]
		if !ok {
			m.order = m.order[1:]
			continue
		}
		if !isTerminalSubagentJobStatus(job.Status) {
			return
		}
		delete(m.jobs, oldestID)
		delete(m.cancels, oldestID)
		m.order = m.order[1:]
	}
}

type subagentManagerPersistedState struct {
	Next  int           `json:"next"`
	Order []string      `json:"order,omitempty"`
	Jobs  []SubagentJob `json:"jobs,omitempty"`
}

func (m *InMemorySubagentManager) loadPersistedJobs() {
	if m == nil || strings.TrimSpace(m.storagePath) == "" {
		return
	}
	raw, err := os.ReadFile(m.storagePath)
	if err != nil {
		return
	}
	var state subagentManagerPersistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return
	}
	m.next = state.Next
	m.order = append([]string(nil), state.Order...)
	for _, job := range state.Jobs {
		if strings.TrimSpace(job.JobID) == "" {
			continue
		}
		m.jobs[job.JobID] = job
	}
	if len(m.order) == 0 && len(m.jobs) > 0 {
		for jobID := range m.jobs {
			m.order = append(m.order, jobID)
		}
	}
}

func (m *InMemorySubagentManager) persistLocked() {
	if m == nil || strings.TrimSpace(m.storagePath) == "" {
		return
	}
	state := subagentManagerPersistedState{
		Next:  m.next,
		Order: append([]string(nil), m.order...),
		Jobs:  make([]SubagentJob, 0, len(m.order)),
	}
	for _, jobID := range m.order {
		job, ok := m.jobs[jobID]
		if !ok {
			continue
		}
		state.Jobs = append(state.Jobs, job)
	}
	if err := os.MkdirAll(filepath.Dir(m.storagePath), 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(m.storagePath, raw, 0o600)
}

func isTerminalSubagentJobStatus(status SubagentJobStatus) bool {
	return status == SubagentJobStatusCompleted || status == SubagentJobStatusFailed || status == SubagentJobStatusCancelled
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
