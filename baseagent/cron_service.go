package baseagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type CronJob struct {
	ID         string    `json:"id"`
	SessionKey string    `json:"sessionKey"`
	Prompt     string    `json:"prompt"`
	NextRunAt  time.Time `json:"nextRunAt"`
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

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	if !job.NextRunAt.After(time.Now().UTC()) && s.execute != nil {
		if err := s.execute(ctx, job); err != nil {
			return CronJob{}, err
		}
	}
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
