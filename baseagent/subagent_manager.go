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
	SubagentJobStatusCompleted SubagentJobStatus = "completed"
	SubagentJobStatusCancelled SubagentJobStatus = "cancelled"
	SubagentJobStatusFailed    SubagentJobStatus = "failed"
)

type SubagentJob struct {
	JobID     string
	Task      string
	Status    SubagentJobStatus
	Summary   string
	Result    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubagentExecutor func(context.Context, SubagentRequest) (string, error)

type SubagentManager interface {
	SubagentSpawner
	Job(ctx context.Context, jobID string) (SubagentJob, error)
	RecentJobs(ctx context.Context, limit int) []SubagentJob
	Cancel(ctx context.Context, jobID string) error
}

type InMemorySubagentManager struct {
	mu          sync.RWMutex
	next        int
	jobs        map[string]SubagentJob
	order       []string
	maxHistory  int
	cancels     map[string]context.CancelFunc
	executor    SubagentExecutor
	storagePath string
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
		jobs:        map[string]SubagentJob{},
		order:       nil,
		maxHistory:  32,
		cancels:     map[string]context.CancelFunc{},
		executor:    executor,
		storagePath: strings.TrimSpace(storagePath),
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

	now := time.Now().UTC()

	m.mu.Lock()
	m.next++
	jobID := fmt.Sprintf("subagent-%d", m.next)
	job := SubagentJob{
		JobID:     jobID,
		Task:      task,
		Status:    SubagentJobStatusQueued,
		Summary:   task,
		CreatedAt: now,
		UpdatedAt: now,
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
		JobID:   job.JobID,
		Status:  string(job.Status),
		Summary: job.Summary,
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

	result, err := m.executor(ctx, req)

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
	return job, nil
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
		jobs = append(jobs, job)
	}
	return jobs
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
