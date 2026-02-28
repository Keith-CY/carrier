package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type remoteAlertDigest struct {
	Active  bool
	Level   string
	Count   int
	Reasons string
}

type remoteAlertWatchdog struct {
	webhookURL string
	interval   time.Duration
	cooldown   time.Duration
	client     *http.Client
	now        func() time.Time
	post       func(context.Context, *http.Client, string, []byte) error

	initialized bool
	lastDigest  remoteAlertDigest
	lastSentAt  time.Time
}

func startRemoteAlertWatchdog(ctx context.Context, cfg *GatewayConfig) {
	if cfg == nil {
		return
	}
	webhookURL := strings.TrimSpace(cfg.RemoteAlertWebhookURL)
	if webhookURL == "" {
		return
	}
	interval := cfg.RemoteAlertInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	cooldown := cfg.RemoteAlertCooldown
	if cooldown < 0 {
		cooldown = 0
	}

	watchdog := &remoteAlertWatchdog{
		webhookURL: webhookURL,
		interval:   interval,
		cooldown:   cooldown,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		now:  time.Now,
		post: defaultRemoteAlertPost,
	}
	go watchdog.run(ctx)
	log.Printf("[gateway] remote alert watchdog enabled (interval=%s cooldown=%s)", interval.String(), cooldown.String())
}

func (w *remoteAlertWatchdog) run(ctx context.Context) {
	w.tick(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *remoteAlertWatchdog) tick(ctx context.Context) {
	if w == nil || strings.TrimSpace(w.webhookURL) == "" {
		return
	}
	now := w.now()
	snapshot := remoteMetrics.snapshot()
	digest := digestRemoteAlertSnapshot(snapshot)
	send, reason, previous, hasPrevious := w.shouldSend(digest, now)
	if !send {
		return
	}
	payload, err := marshalRemoteAlertPayload(snapshot, digest, previous, hasPrevious, reason, now)
	if err != nil {
		log.Printf("[gateway] remote alert webhook marshal failed: %v", err)
		return
	}
	if err := w.post(ctx, w.client, w.webhookURL, payload); err != nil {
		log.Printf("[gateway] remote alert webhook send failed: %v", err)
		return
	}
	w.markSent(digest, now)
}

func (w *remoteAlertWatchdog) shouldSend(nowDigest remoteAlertDigest, now time.Time) (bool, string, remoteAlertDigest, bool) {
	if !w.initialized {
		if !nowDigest.Active {
			w.initialized = true
			w.lastDigest = nowDigest
			return false, "", remoteAlertDigest{}, false
		}
		return true, "initial-active", remoteAlertDigest{}, false
	}

	previous := w.lastDigest
	if nowDigest != w.lastDigest {
		return true, "state-change", previous, true
	}
	if nowDigest.Active && w.cooldown > 0 && !w.lastSentAt.IsZero() && now.Sub(w.lastSentAt) >= w.cooldown {
		return true, "cooldown", previous, true
	}
	return false, "", previous, true
}

func (w *remoteAlertWatchdog) markSent(nowDigest remoteAlertDigest, now time.Time) {
	w.initialized = true
	w.lastDigest = nowDigest
	w.lastSentAt = now
}

func digestRemoteAlertSnapshot(snapshot remoteMetricsSnapshot) remoteAlertDigest {
	level := strings.TrimSpace(snapshot.Alerts.Level)
	if level == "" {
		level = "none"
	}
	return remoteAlertDigest{
		Active:  snapshot.Alerts.Active,
		Level:   level,
		Count:   snapshot.Alerts.Count,
		Reasons: strings.Join(snapshot.Rollout.Reasons, "\n"),
	}
}

func marshalRemoteAlertPayload(
	snapshot remoteMetricsSnapshot,
	nowDigest remoteAlertDigest,
	previous remoteAlertDigest,
	hasPrevious bool,
	trigger string,
	now time.Time,
) ([]byte, error) {
	state := "active"
	if !nowDigest.Active {
		state = "resolved"
	}
	payload := map[string]interface{}{
		"event":     "remote_alert",
		"state":     state,
		"trigger":   strings.TrimSpace(trigger),
		"timestamp": now.UTC().Format(time.RFC3339),
		"alerts":    snapshot.Alerts,
		"rollout":   snapshot.Rollout,
		"totals":    snapshot.Totals,
	}
	if hasPrevious {
		payload["previous"] = map[string]interface{}{
			"active":  previous.Active,
			"level":   previous.Level,
			"count":   previous.Count,
			"reasons": previous.Reasons,
		}
	}
	return json.Marshal(payload)
}

func defaultRemoteAlertPost(ctx context.Context, client *http.Client, webhookURL string, payload []byte) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(webhookURL), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with status %d", resp.StatusCode)
	}
	return nil
}
