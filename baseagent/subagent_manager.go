package baseagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SubagentJobStatus string

const (
	SubagentJobStatusQueued    SubagentJobStatus = "queued"
	SubagentJobStatusRunning   SubagentJobStatus = "running"
	SubagentJobStatusCompleted SubagentJobStatus = "completed"
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
}

type InMemorySubagentManager struct {
	mu       sync.RWMutex
	next     int
	jobs     map[string]SubagentJob
	executor SubagentExecutor
}

func NewInMemorySubagentManager(executor SubagentExecutor) *InMemorySubagentManager {
	if executor == nil {
		executor = func(_ context.Context, req SubagentRequest) (string, error) {
			task := strings.TrimSpace(req.Task)
			if task == "" {
				task = "delegated task"
			}
			return "delegated task accepted: " + task, nil
		}
	}
	return &InMemorySubagentManager{
		jobs:     map[string]SubagentJob{},
		executor: executor,
	}
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
	m.mu.Unlock()

	execCtx := context.Background()
	if ctx != nil {
		execCtx = context.WithoutCancel(ctx)
	}
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
		job.Status = SubagentJobStatusFailed
		job.Error = strings.TrimSpace(err.Error())
	} else {
		job.Status = SubagentJobStatusCompleted
		job.Result = strings.TrimSpace(result)
	}
	m.jobs[jobID] = job
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
