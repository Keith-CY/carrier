package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	WebhookEventAgentInstalled = "agent.installed"
	WebhookEventAgentStarted   = "agent.started"
	WebhookEventAgentStopped   = "agent.stopped"
	WebhookEventAgentCrashed   = "agent.crashed"
	WebhookEventAgentUpgraded  = "agent.upgraded"
)

type WebhookEvent struct {
	Type      string    `json:"type"`
	AgentID   string    `json:"agentId"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type WebhookManager struct {
	url     string
	client  *http.Client
	filters map[string]bool
	enabled bool
	sleep   func(time.Duration)
}

func NewWebhookManager(url string, events []string) *WebhookManager {
	filters := map[string]bool{}
	for _, event := range events {
		e := strings.TrimSpace(event)
		if e != "" {
			filters[e] = true
		}
	}
	return &WebhookManager{
		url:     strings.TrimSpace(url),
		client:  &http.Client{Timeout: 10 * time.Second},
		filters: filters,
		enabled: strings.TrimSpace(url) != "",
		sleep:   time.Sleep,
	}
}

func (m *WebhookManager) FireEvent(event WebhookEvent) error {
	if m == nil || !m.enabled {
		return nil
	}
	if len(m.filters) > 0 && !m.filters[strings.TrimSpace(event.Type)] {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, m.url, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := m.client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			err = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
		lastErr = err
		if attempt < 2 {
			m.sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
		}
	}
	return lastErr
}
