package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type CronJob struct {
	ID          string     `json:"id"`
	AgentID     string     `json:"agentId,omitempty"`
	SessionKey  string     `json:"sessionKey"`
	Prompt      string     `json:"prompt"`
	NextRunAt   time.Time  `json:"nextRunAt"`
	LastRunAt   *time.Time `json:"lastRunAt,omitempty"`
	LastResult  string     `json:"lastResult,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	Paused      bool       `json:"paused,omitempty"`
	PausedAt    *time.Time `json:"pausedAt,omitempty"`
	CancelledAt *time.Time `json:"cancelledAt,omitempty"`
	History     []CronRun  `json:"history,omitempty"`
}

type CronRun struct {
	RanAt    time.Time `json:"ranAt"`
	Trigger  string    `json:"trigger,omitempty"`
	Result   string    `json:"result,omitempty"`
	Error    string    `json:"error,omitempty"`
	Prompt   string    `json:"prompt,omitempty"`
	AgentID  string    `json:"agentId,omitempty"`
}

type CronService struct {
	mu      sync.Mutex
	jobs    map[string]CronJob
	execute func(context.Context, CronJob) error
}

func NewCronService(execute func(context.Context, CronJob) error) *CronService {
	return &CronService{
		jobs:    map[string]CronJob{},
		execute: execute,
	}
}

func (s *CronService) Schedule(ctx context.Context, job CronJob) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	job.SessionKey = strings.TrimSpace(job.SessionKey)
	job.Prompt = strings.TrimSpace(job.Prompt)
	if job.SessionKey == "" {
		return CronJob{}, fmt.Errorf("session key is required")
	}
	if job.Prompt == "" {
		return CronJob{}, fmt.Errorf("prompt is required")
	}
	if job.ID == "" {
		job.ID = fmt.Sprintf("cron-%d", time.Now().UTC().UnixNano())
	}
	if job.NextRunAt.IsZero() {
		job.NextRunAt = time.Now().UTC()
	}
	if strings.TrimSpace(job.LastResult) == "" {
		job.LastResult = "scheduled"
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	if !job.NextRunAt.After(time.Now().UTC()) && s.execute != nil {
		updated, err := s.executeAndRecord(ctx, job, "schedule")
		if err != nil {
			return CronJob{}, err
		}
		job = updated
	}
	return job, nil
}

func (s *CronService) List(sessionKey string) []CronJob {
	if s == nil {
		return nil
	}
	trimmedSession := strings.TrimSpace(sessionKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CronJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		if trimmedSession != "" && !strings.EqualFold(strings.TrimSpace(job.SessionKey), trimmedSession) {
			continue
		}
		out = append(out, job)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].NextRunAt.Equal(out[j].NextRunAt) {
			return out[i].NextRunAt.Before(out[j].NextRunAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *CronService) Cancel(jobID string) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return CronJob{}, fmt.Errorf("cron job id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[trimmedID]
	if !ok {
		return CronJob{}, fmt.Errorf("cron job %s not found", trimmedID)
	}
	now := time.Now().UTC()
	job.CancelledAt = &now
	job.LastResult = "cancelled"
	job.LastError = ""
	s.jobs[trimmedID] = job
	return job, nil
}

func (s *CronService) Pause(jobID string) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return CronJob{}, fmt.Errorf("cron job id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[trimmedID]
	if !ok {
		return CronJob{}, fmt.Errorf("cron job %s not found", trimmedID)
	}
	now := time.Now().UTC()
	job.Paused = true
	job.PausedAt = &now
	job.LastResult = "paused"
	job.LastError = ""
	s.jobs[trimmedID] = job
	return job, nil
}

func (s *CronService) Resume(jobID string) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return CronJob{}, fmt.Errorf("cron job id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[trimmedID]
	if !ok {
		return CronJob{}, fmt.Errorf("cron job %s not found", trimmedID)
	}
	job.Paused = false
	job.PausedAt = nil
	job.LastResult = "resumed"
	job.LastError = ""
	s.jobs[trimmedID] = job
	return job, nil
}

func (s *CronService) RunNow(ctx context.Context, jobID string) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	trimmedID := strings.TrimSpace(jobID)
	if trimmedID == "" {
		return CronJob{}, fmt.Errorf("cron job id is required")
	}
	s.mu.Lock()
	job, ok := s.jobs[trimmedID]
	s.mu.Unlock()
	if !ok {
		return CronJob{}, fmt.Errorf("cron job %s not found", trimmedID)
	}
	if job.Paused {
		return CronJob{}, fmt.Errorf("cron job %s is paused", trimmedID)
	}
	return s.executeAndRecord(ctx, job, "manual")
}

func (s *CronService) executeAndRecord(ctx context.Context, job CronJob, trigger string) (CronJob, error) {
	if s == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	if s.execute != nil {
		if err := s.execute(ctx, job); err != nil {
			now := time.Now().UTC()
			job.LastRunAt = &now
			job.LastResult = "failed"
			job.LastError = err.Error()
			job.History = append(job.History, CronRun{
				RanAt:   now,
				Trigger: strings.TrimSpace(trigger),
				Result:  "failed",
				Error:   err.Error(),
				Prompt:  job.Prompt,
				AgentID: job.AgentID,
			})
			s.mu.Lock()
			s.jobs[job.ID] = job
			s.mu.Unlock()
			return CronJob{}, err
		}
	}
	now := time.Now().UTC()
	job.LastRunAt = &now
	job.LastResult = "succeeded"
	job.LastError = ""
	job.History = append(job.History, CronRun{
		RanAt:   now,
		Trigger: strings.TrimSpace(trigger),
		Result:  "succeeded",
		Prompt:  job.Prompt,
		AgentID: job.AgentID,
	})
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	return job, nil
}

func cronChatRequestForSessionKey(sessionKey, prompt string) ChatRequest {
	sessionKey = strings.TrimSpace(sessionKey)
	provider := "cron"
	chatID := sessionKey
	if parts := strings.SplitN(sessionKey, ":", 2); len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		provider = strings.TrimSpace(parts[0])
		chatID = strings.TrimSpace(parts[1])
	}
	return ChatRequest{
		Provider: provider,
		ChatID:   chatID,
		Message:  strings.TrimSpace(prompt),
	}
}
