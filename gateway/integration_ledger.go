package gateway

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"carrier/shared/integration"

	"github.com/google/uuid"
)

func appendIntegrationEvent(executionID, attemptID, eventType string, payload any) (integration.Event, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return integration.Event{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return integration.Event{}, fmt.Errorf("begin integration event tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	event, err := appendIntegrationEventTx(tx, executionID, attemptID, eventType, payload)
	if err != nil {
		return integration.Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return integration.Event{}, fmt.Errorf("commit integration event tx: %w", err)
	}
	return event, nil
}

func appendIntegrationEventTx(tx *sql.Tx, executionID, attemptID, eventType string, payload any) (integration.Event, error) {
	sequence, err := nextIntegrationEventSequenceTx(tx, executionID)
	if err != nil {
		return integration.Event{}, err
	}
	payloadJSON := ""
	if payload != nil {
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return integration.Event{}, fmt.Errorf("marshal integration event payload: %w", marshalErr)
		}
		payloadJSON = string(raw)
	}
	event := integration.Event{
		ID:          "evt_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ExecutionID: strings.TrimSpace(executionID),
		AttemptID:   strings.TrimSpace(attemptID),
		Sequence:    sequence,
		EventType:   strings.TrimSpace(eventType),
		PayloadJSON: payloadJSON,
		CreatedAt:   nowTimestamp(),
	}
	if event.ExecutionID == "" {
		return integration.Event{}, fmt.Errorf("event execution id is required")
	}
	if event.EventType == "" {
		return integration.Event{}, fmt.Errorf("event type is required")
	}
	if _, err := tx.Exec(`
		INSERT INTO integration_events (id, execution_id, attempt_id, sequence, event_type, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.ExecutionID, event.AttemptID, event.Sequence, event.EventType, event.PayloadJSON, event.CreatedAt); err != nil {
		return integration.Event{}, fmt.Errorf("insert integration event: %w", err)
	}
	if bindingID, callbackURL, err := lookupExecutionBindingCallbackTx(tx, event.ExecutionID); err != nil {
		return integration.Event{}, err
	} else if strings.TrimSpace(callbackURL) != "" {
		delivery := integration.CallbackDelivery{
			ID:            "delivery_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			EventID:       event.ID,
			ExecutionID:   event.ExecutionID,
			BindingID:     bindingID,
			CallbackURL:   callbackURL,
			Status:        "pending",
			AttemptCount:  0,
			NextAttemptAt: event.CreatedAt,
			CreatedAt:     event.CreatedAt,
			UpdatedAt:     event.CreatedAt,
		}
		if _, err := tx.Exec(`
			INSERT INTO integration_callback_deliveries (
				id, event_id, execution_id, binding_id, callback_url, status, attempt_count, last_error, next_attempt_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, delivery.ID, delivery.EventID, delivery.ExecutionID, delivery.BindingID, delivery.CallbackURL, delivery.Status, delivery.AttemptCount, delivery.LastError, delivery.NextAttemptAt, delivery.CreatedAt, delivery.UpdatedAt); err != nil {
			return integration.Event{}, fmt.Errorf("insert integration callback delivery: %w", err)
		}
	}
	return event, nil
}

func listIntegrationEventsByExecutionID(executionID string) ([]integration.Event, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, execution_id, attempt_id, sequence, event_type, payload_json, created_at
		FROM integration_events
		WHERE execution_id = ?
		ORDER BY sequence ASC
	`, strings.TrimSpace(executionID))
	if err != nil {
		return nil, fmt.Errorf("query integration events: %w", err)
	}
	defer rows.Close()
	events := []integration.Event{}
	for rows.Next() {
		var event integration.Event
		if err := rows.Scan(&event.ID, &event.ExecutionID, &event.AttemptID, &event.Sequence, &event.EventType, &event.PayloadJSON, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan integration event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate integration events: %w", err)
	}
	return events, nil
}

func appendIntegrationEventByOrchestratorExecution(orchestratorExecutionID, eventType string, payload any) error {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return err
	}
	defer db.Close()
	row := db.QueryRow(`
		SELECT id, binding_id, adapter, account, external_execution_id, orchestrator_execution_id, idempotency_key,
			state, goal, input, requested_provider, failure_category, failure_code, current_attempt_id,
			created_at, started_at, completed_at, updated_at
		FROM integration_executions
		WHERE orchestrator_execution_id = ?
	`, strings.TrimSpace(orchestratorExecutionID))
	exec, found, err := scanIntegrationExecution(row)
	if err != nil || !found {
		return err
	}
	attempt, _, err := loadCurrentAttemptByExecutionID(db, exec.ID)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin integration lifecycle event tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := appendIntegrationEventTx(tx, exec.ID, attempt.ID, eventType, payload); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit integration lifecycle event tx: %w", err)
	}
	return nil
}

func upsertIntegrationUsageProof(proof integration.UsageProof) (integration.UsageProof, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return integration.UsageProof{}, err
	}
	defer db.Close()
	if strings.TrimSpace(proof.ID) == "" {
		proof.ID = "proof_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	if strings.TrimSpace(proof.CreatedAt) == "" {
		proof.CreatedAt = nowTimestamp()
	}
	if _, err := db.Exec(`
		INSERT INTO integration_usage_proofs (id, execution_id, proof_ref, meter_ref, usage_kind, amount_cents, digest, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			execution_id=excluded.execution_id,
			proof_ref=excluded.proof_ref,
			meter_ref=excluded.meter_ref,
			usage_kind=excluded.usage_kind,
			amount_cents=excluded.amount_cents,
			digest=excluded.digest
	`, proof.ID, proof.ExecutionID, proof.ProofRef, proof.MeterRef, proof.UsageKind, proof.AmountCents, proof.Digest, proof.CreatedAt); err != nil {
		return integration.UsageProof{}, fmt.Errorf("upsert integration usage proof: %w", err)
	}
	return proof, nil
}

func upsertIntegrationArtifactRef(ref integration.ArtifactRef) (integration.ArtifactRef, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return integration.ArtifactRef{}, err
	}
	defer db.Close()
	if strings.TrimSpace(ref.ID) == "" {
		ref.ID = "artifact_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	if strings.TrimSpace(ref.CreatedAt) == "" {
		ref.CreatedAt = nowTimestamp()
	}
	if _, err := db.Exec(`
		INSERT INTO integration_artifact_refs (id, execution_id, artifact_ref, kind, name, url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			execution_id=excluded.execution_id,
			artifact_ref=excluded.artifact_ref,
			kind=excluded.kind,
			name=excluded.name,
			url=excluded.url
	`, ref.ID, ref.ExecutionID, ref.ArtifactRef, ref.Kind, ref.Name, ref.URL, ref.CreatedAt); err != nil {
		return integration.ArtifactRef{}, fmt.Errorf("upsert integration artifact ref: %w", err)
	}
	return ref, nil
}

func listIntegrationUsageProofsByExecutionID(executionID string) ([]integration.UsageProof, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()
	db, err := openIntegrationDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, execution_id, proof_ref, meter_ref, usage_kind, amount_cents, digest, created_at
		FROM integration_usage_proofs
		WHERE execution_id = ?
		ORDER BY created_at ASC, id ASC
	`, strings.TrimSpace(executionID))
	if err != nil {
		return nil, fmt.Errorf("query integration usage proofs: %w", err)
	}
	defer rows.Close()
	out := []integration.UsageProof{}
	for rows.Next() {
		var proof integration.UsageProof
		if err := rows.Scan(&proof.ID, &proof.ExecutionID, &proof.ProofRef, &proof.MeterRef, &proof.UsageKind, &proof.AmountCents, &proof.Digest, &proof.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan integration usage proof: %w", err)
		}
		out = append(out, proof)
	}
	return out, rows.Err()
}

func listIntegrationArtifactRefsByExecutionID(executionID string) ([]integration.ArtifactRef, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()
	db, err := openIntegrationDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, execution_id, artifact_ref, kind, name, url, created_at
		FROM integration_artifact_refs
		WHERE execution_id = ?
		ORDER BY created_at ASC, id ASC
	`, strings.TrimSpace(executionID))
	if err != nil {
		return nil, fmt.Errorf("query integration artifact refs: %w", err)
	}
	defer rows.Close()
	out := []integration.ArtifactRef{}
	for rows.Next() {
		var ref integration.ArtifactRef
		if err := rows.Scan(&ref.ID, &ref.ExecutionID, &ref.ArtifactRef, &ref.Kind, &ref.Name, &ref.URL, &ref.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan integration artifact ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func syncIntegrationArtifactRefsForExecution(exec integration.Execution, orchestrator OrchestratorExecution) error {
	for _, artifact := range orchestrator.Outcome.Artifacts {
		key := strings.TrimSpace(artifact.ID)
		if key == "" {
			key = strings.TrimSpace(artifact.Name)
		}
		if key == "" {
			continue
		}
		sum := sha256.Sum256([]byte(exec.ID + "|" + key))
		_, err := upsertIntegrationArtifactRef(integration.ArtifactRef{
			ID:          "artref_" + hex.EncodeToString(sum[:])[:16],
			ExecutionID: exec.ID,
			ArtifactRef: "artifact://" + exec.ID + "/" + key,
			Kind:        artifact.Kind,
			Name:        firstNonEmpty(artifact.Name, artifact.ID),
			URL:         artifact.DownloadURL,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func listIntegrationCallbackDeliveriesByExecutionID(executionID string) ([]integration.CallbackDelivery, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()
	db, err := openIntegrationDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, event_id, execution_id, binding_id, callback_url, status, attempt_count, last_error, next_attempt_at, created_at, updated_at
		FROM integration_callback_deliveries
		WHERE execution_id = ?
		ORDER BY created_at ASC, id ASC
	`, strings.TrimSpace(executionID))
	if err != nil {
		return nil, fmt.Errorf("query integration callback deliveries: %w", err)
	}
	defer rows.Close()
	out := []integration.CallbackDelivery{}
	for rows.Next() {
		var delivery integration.CallbackDelivery
		if err := rows.Scan(&delivery.ID, &delivery.EventID, &delivery.ExecutionID, &delivery.BindingID, &delivery.CallbackURL, &delivery.Status, &delivery.AttemptCount, &delivery.LastError, &delivery.NextAttemptAt, &delivery.CreatedAt, &delivery.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan integration callback delivery: %w", err)
		}
		out = append(out, delivery)
	}
	return out, rows.Err()
}

func nextIntegrationEventSequenceTx(tx *sql.Tx, executionID string) (int64, error) {
	row := tx.QueryRow(`SELECT COALESCE(MAX(sequence), 0) FROM integration_events WHERE execution_id = ?`, strings.TrimSpace(executionID))
	var maxSequence int64
	if err := row.Scan(&maxSequence); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("query integration event sequence: %w", err)
	}
	return maxSequence + 1, nil
}

func lookupExecutionBindingCallbackTx(tx *sql.Tx, executionID string) (string, string, error) {
	row := tx.QueryRow(`
		SELECT e.binding_id, b.callback_url
		FROM integration_executions e
		JOIN integration_bindings b ON b.id = e.binding_id
		WHERE e.id = ?
	`, strings.TrimSpace(executionID))
	var bindingID string
	var callbackURL string
	if err := row.Scan(&bindingID, &callbackURL); errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	} else if err != nil {
		return "", "", fmt.Errorf("lookup integration callback target: %w", err)
	}
	return bindingID, callbackURL, nil
}
