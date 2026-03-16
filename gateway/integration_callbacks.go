package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"carrier/shared/integration"
)

const (
	integrationCallbackStatusPending    = "pending"
	integrationCallbackStatusDelivering = "delivering"
	integrationCallbackStatusDelivered  = "delivered"
	integrationCallbackStatusFailed     = "failed"
)

var (
	integrationCallbackHTTPClient = &http.Client{Timeout: 10 * time.Second}
	integrationCallbackStartOnce  sync.Once
)

type integrationCallbackJob struct {
	Delivery integration.CallbackDelivery
	Event    integration.Event
	Binding  integration.Binding
}

type integrationCallbackEnvelope struct {
	EventID     string      `json:"eventId"`
	Sequence    int64       `json:"sequence"`
	EventType   string      `json:"eventType"`
	BindingID   string      `json:"bindingId"`
	ExecutionID string      `json:"carrierExecutionId"`
	AttemptID   string      `json:"attemptId,omitempty"`
	CreatedAt   string      `json:"createdAt,omitempty"`
	Payload     interface{} `json:"payload,omitempty"`
}

type integrationCallbackResponse struct {
	Accepted          *bool                                 `json:"accepted,omitempty"`
	RecommendedAction *integrationCallbackRecommendedAction `json:"recommendedAction,omitempty"`
}

type integrationCallbackRecommendedAction struct {
	Type   integration.ActionType `json:"type"`
	Reason string                 `json:"reason,omitempty"`
}

func startIntegrationCallbackDispatcher(ctx context.Context, pollInterval time.Duration) {
	integrationCallbackStartOnce.Do(func() {
		go runIntegrationCallbackDispatcher(ctx, pollInterval)
	})
}

func runIntegrationCallbackDispatcher(ctx context.Context, pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := dispatchPendingIntegrationCallbacks(ctx, integrationCallbackHTTPClient, 16); err != nil {
				log.Printf("[gateway/integrations] callback dispatch error: %v", err)
			}
		}
	}
}

func dispatchPendingIntegrationCallbacks(ctx context.Context, client *http.Client, limit int) (int, error) {
	if limit <= 0 {
		limit = 16
	}
	if client == nil {
		client = integrationCallbackHTTPClient
	}
	jobs, err := claimPendingIntegrationCallbackDeliveries(limit)
	if err != nil {
		return 0, err
	}
	delivered := 0
	var firstErr error
	for _, job := range jobs {
		if err := deliverIntegrationCallbackJob(ctx, client, job); err != nil {
			if updateErr := updateIntegrationCallbackDeliveryResult(job.Delivery.ID, job.Delivery.AttemptCount, integrationCallbackStatusFailed, err.Error()); updateErr != nil && firstErr == nil {
				firstErr = updateErr
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := updateIntegrationCallbackDeliveryResult(job.Delivery.ID, job.Delivery.AttemptCount, integrationCallbackStatusDelivered, ""); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		delivered++
	}
	return delivered, firstErr
}

func deliverIntegrationCallbackJob(ctx context.Context, client *http.Client, job integrationCallbackJob) error {
	if strings.TrimSpace(job.Delivery.CallbackURL) == "" {
		return fmt.Errorf("callback url is not configured")
	}
	if strings.TrimSpace(job.Binding.CallbackSecret) == "" {
		return fmt.Errorf("callback secret is not configured")
	}
	var payload interface{}
	if strings.TrimSpace(job.Event.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(job.Event.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("decode callback payload: %w", err)
		}
	}
	envelope := integrationCallbackEnvelope{
		EventID:     job.Event.ID,
		Sequence:    job.Event.Sequence,
		EventType:   job.Event.EventType,
		BindingID:   job.Delivery.BindingID,
		ExecutionID: job.Event.ExecutionID,
		AttemptID:   job.Event.AttemptID,
		CreatedAt:   job.Event.CreatedAt,
		Payload:     payload,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal callback envelope: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.Delivery.CallbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build callback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Carrier-Key-Id", strings.TrimSpace(job.Binding.CallbackKeyID))
	req.Header.Set("X-Carrier-Event-Id", job.Event.ID)
	req.Header.Set("X-Carrier-Event-Sequence", strconv.FormatInt(job.Event.Sequence, 10))
	req.Header.Set("X-Carrier-Signature", signIntegrationCallbackBody(job.Binding.CallbackSecret, body))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver callback: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read callback response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("callback returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	callbackResp := integrationCallbackResponse{}
	if len(bytes.TrimSpace(respBody)) > 0 {
		if err := json.Unmarshal(respBody, &callbackResp); err != nil {
			return fmt.Errorf("decode callback response: %w", err)
		}
	}
	if callbackResp.Accepted != nil && !*callbackResp.Accepted {
		return fmt.Errorf("callback rejected event delivery")
	}
	if action := callbackResp.RecommendedAction; action != nil && action.Type != "" {
		if _, status, err := applyIntegrationAction(job.Event.ExecutionID, integration.ActionRequest{
			Type:           action.Type,
			IdempotencyKey: "callback:" + job.Event.ID + ":" + string(action.Type),
			Reason:         strings.TrimSpace(action.Reason),
		}); err != nil {
			return fmt.Errorf("apply callback recommended action (%d): %w", status, err)
		}
	}
	return nil
}

func signIntegrationCallbackBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func claimPendingIntegrationCallbackDeliveries(limit int) ([]integrationCallbackJob, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin callback delivery claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`
		SELECT
			d.id, d.event_id, d.execution_id, d.binding_id, d.callback_url, d.status, d.attempt_count, d.last_error, d.next_attempt_at, d.created_at, d.updated_at,
			e.id, e.execution_id, e.attempt_id, e.sequence, e.event_type, e.payload_json, e.created_at,
			b.id, b.adapter, b.account, b.callback_url, b.callback_key_id, b.callback_secret, b.host_id, b.agent_id, b.backend, b.workspace_root, b.status, b.capabilities_json, b.created_at, b.updated_at
		FROM integration_callback_deliveries d
		JOIN integration_events e ON e.id = d.event_id
		JOIN integration_bindings b ON b.id = d.binding_id
		WHERE d.status IN (?, ?)
			AND (d.next_attempt_at = '' OR d.next_attempt_at <= ?)
			AND NOT EXISTS (
				SELECT 1
				FROM integration_callback_deliveries d_prev
				JOIN integration_events e_prev ON e_prev.id = d_prev.event_id
				WHERE d_prev.execution_id = d.execution_id
					AND d_prev.status != ?
					AND e_prev.sequence < e.sequence
			)
		ORDER BY d.created_at ASC, d.id ASC
		LIMIT ?
	`, integrationCallbackStatusPending, integrationCallbackStatusFailed, nowTimestamp(), integrationCallbackStatusDelivered, limit)
	if err != nil {
		return nil, fmt.Errorf("query callback deliveries: %w", err)
	}
	defer rows.Close()

	now := nowTimestamp()
	out := []integrationCallbackJob{}
	for rows.Next() {
		var (
			job     integrationCallbackJob
			capsRaw string
		)
		if err := rows.Scan(
			&job.Delivery.ID, &job.Delivery.EventID, &job.Delivery.ExecutionID, &job.Delivery.BindingID, &job.Delivery.CallbackURL, &job.Delivery.Status, &job.Delivery.AttemptCount, &job.Delivery.LastError, &job.Delivery.NextAttemptAt, &job.Delivery.CreatedAt, &job.Delivery.UpdatedAt,
			&job.Event.ID, &job.Event.ExecutionID, &job.Event.AttemptID, &job.Event.Sequence, &job.Event.EventType, &job.Event.PayloadJSON, &job.Event.CreatedAt,
			&job.Binding.ID, &job.Binding.Adapter, &job.Binding.Account, &job.Binding.CallbackURL, &job.Binding.CallbackKeyID, &job.Binding.CallbackSecret, &job.Binding.Target.HostID, &job.Binding.Target.AgentID, &job.Binding.Target.Backend, &job.Binding.Target.WorkspaceRoot, &job.Binding.Status, &capsRaw, &job.Binding.CreatedAt, &job.Binding.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan callback delivery job: %w", err)
		}
		if strings.TrimSpace(capsRaw) != "" {
			if err := json.Unmarshal([]byte(capsRaw), &job.Binding.Capabilities); err != nil {
				return nil, fmt.Errorf("decode callback binding capabilities: %w", err)
			}
		}
		job.Delivery.Status = integrationCallbackStatusDelivering
		job.Delivery.AttemptCount++
		job.Delivery.NextAttemptAt = ""
		job.Delivery.UpdatedAt = now
		if _, err := tx.Exec(`
			UPDATE integration_callback_deliveries
			SET status = ?, attempt_count = ?, next_attempt_at = ?, updated_at = ?
			WHERE id = ?
		`, job.Delivery.Status, job.Delivery.AttemptCount, job.Delivery.NextAttemptAt, job.Delivery.UpdatedAt, job.Delivery.ID); err != nil {
			return nil, fmt.Errorf("mark callback delivery delivering: %w", err)
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate callback deliveries: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit callback delivery claim tx: %w", err)
	}
	return out, nil
}

func updateIntegrationCallbackDeliveryResult(deliveryID string, attemptCount int, status, lastError string) error {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return err
	}
	defer db.Close()
	nextAttemptAt := ""
	if strings.EqualFold(strings.TrimSpace(status), integrationCallbackStatusFailed) {
		backoffSeconds := 1
		if attemptCount > 1 {
			backoffSeconds = 1 << min(attemptCount-1, 6)
		}
		nextAttemptAt = time.Now().UTC().Add(time.Duration(backoffSeconds) * time.Second).Format(time.RFC3339Nano)
	}
	_, err = db.Exec(`
		UPDATE integration_callback_deliveries
		SET status = ?, last_error = ?, next_attempt_at = ?, updated_at = ?
		WHERE id = ?
	`, strings.TrimSpace(status), strings.TrimSpace(lastError), nextAttemptAt, nowTimestamp(), strings.TrimSpace(deliveryID))
	if err != nil {
		return fmt.Errorf("update callback delivery result: %w", err)
	}
	return nil
}

func replayIntegrationCallbackDeliveries(executionID string, fromSequence int64, eventID string) (int, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return 0, err
	}
	defer db.Close()

	targetSequence := fromSequence
	if targetSequence <= 0 && strings.TrimSpace(eventID) != "" {
		row := db.QueryRow(`
			SELECT sequence
			FROM integration_events
			WHERE execution_id = ? AND id = ?
		`, strings.TrimSpace(executionID), strings.TrimSpace(eventID))
		if scanErr := row.Scan(&targetSequence); scanErr != nil {
			if scanErr == sql.ErrNoRows {
				return 0, fmt.Errorf("integration event %s not found", strings.TrimSpace(eventID))
			}
			return 0, fmt.Errorf("load replay event sequence: %w", scanErr)
		}
	}
	if targetSequence <= 0 {
		return 0, fmt.Errorf("fromSequence or eventId is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin callback replay tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	blocking := tx.QueryRow(`
		SELECT COUNT(1)
		FROM integration_callback_deliveries d
		JOIN integration_events e ON e.id = d.event_id
		WHERE d.execution_id = ? AND e.sequence >= ? AND d.status = ?
	`, strings.TrimSpace(executionID), targetSequence, integrationCallbackStatusDelivering)
	var deliveringCount int
	if err := blocking.Scan(&deliveringCount); err != nil {
		return 0, fmt.Errorf("count callback deliveries in-flight: %w", err)
	}
	if deliveringCount > 0 {
		return 0, fmt.Errorf("callback deliveries are currently in-flight")
	}

	result, err := tx.Exec(`
		UPDATE integration_callback_deliveries
		SET status = ?, last_error = '', next_attempt_at = '', updated_at = ?
		WHERE id IN (
			SELECT d.id
			FROM integration_callback_deliveries d
			JOIN integration_events e ON e.id = d.event_id
			WHERE d.execution_id = ? AND e.sequence >= ?
		)
	`, integrationCallbackStatusPending, nowTimestamp(), strings.TrimSpace(executionID), targetSequence)
	if err != nil {
		return 0, fmt.Errorf("reset callback deliveries for replay: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected for callback replay: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit callback replay tx: %w", err)
	}
	return int(affected), nil
}
