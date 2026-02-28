package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AlertCondition string

const (
	AlertConditionCrashLoop         AlertCondition = "CrashLoop"
	AlertConditionInstallFailure    AlertCondition = "InstallFailure"
	AlertConditionHealthCheckFailed AlertCondition = "HealthCheckFailure"
)

type Alert struct {
	Condition AlertCondition `json:"condition"`
	AgentID   string         `json:"agentId"`
	Details   string         `json:"details"`
	Timestamp time.Time      `json:"timestamp"`
}

type AlertSink interface {
	Send(ctx context.Context, alert Alert) error
}

type WebhookAlertSink struct {
	URL    string
	Client *http.Client
}

func (w WebhookAlertSink) Send(ctx context.Context, alert Alert) error {
	url := strings.TrimSpace(w.URL)
	if url == "" {
		return fmt.Errorf("webhook URL is empty")
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	raw, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alert webhook returned status %d", resp.StatusCode)
	}
	return nil
}

type AlertManager struct {
	enabled     bool
	sink        AlertSink
	dedupWindow time.Duration
	now         func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

func NewAlertManager(enabled bool, sink AlertSink) *AlertManager {
	return &AlertManager{
		enabled:     enabled,
		sink:        sink,
		dedupWindow: 30 * time.Minute,
		now:         time.Now,
		last:        map[string]time.Time{},
	}
}

func (m *AlertManager) Fire(ctx context.Context, condition AlertCondition, agentID, details string) error {
	if m == nil || !m.enabled || m.sink == nil {
		return nil
	}
	now := m.now()
	key := string(condition) + ":" + strings.TrimSpace(agentID)

	m.mu.Lock()
	if prev, ok := m.last[key]; ok && now.Sub(prev) < m.dedupWindow {
		m.mu.Unlock()
		return nil
	}
	m.last[key] = now
	m.mu.Unlock()

	return m.sink.Send(ctx, Alert{
		Condition: condition,
		AgentID:   strings.TrimSpace(agentID),
		Details:   strings.TrimSpace(details),
		Timestamp: now.UTC(),
	})
}
