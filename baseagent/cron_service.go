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
	CancelledAt *time.Time `json:"cancelledAt,omitempty"`
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
		if err := s.execute(ctx, job); err != nil {
			now := time.Now().UTC()
			job.LastRunAt = &now
			job.LastResult = "failed"
			job.LastError = err.Error()
			s.mu.Lock()
			s.jobs[job.ID] = job
			s.mu.Unlock()
			return CronJob{}, err
		}
		now := time.Now().UTC()
		job.LastRunAt = &now
		job.LastResult = "succeeded"
		job.LastError = ""
		s.mu.Lock()
		s.jobs[job.ID] = job
		s.mu.Unlock()
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
