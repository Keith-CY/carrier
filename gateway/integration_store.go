package gateway

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"carrier/shared/integration"

	_ "modernc.org/sqlite"
)

var (
	integrationStoreMu sync.Mutex
)

const integrationTokenPrefix = "cit_"

func upsertIntegrationBinding(binding integration.Binding) (integration.Binding, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	normalized, err := integration.NormalizeBinding(binding)
	if err != nil {
		return integration.Binding{}, err
	}
	now := nowTimestamp()
	existing, err := loadIntegrationBindingByIDLocked(normalized.ID)
	if err != nil {
		return integration.Binding{}, err
	}
	if strings.TrimSpace(existing.ID) == "" {
		normalized.CreatedAt = now
	} else {
		normalized.CreatedAt = existing.CreatedAt
	}
	normalized.UpdatedAt = now

	db, err := openIntegrationDB()
	if err != nil {
		return integration.Binding{}, err
	}
	defer db.Close()

	capsJSON, err := json.Marshal(normalized.Capabilities)
	if err != nil {
		return integration.Binding{}, fmt.Errorf("marshal capabilities: %w", err)
	}
	if _, err := db.Exec(`
		INSERT INTO integration_bindings (
			id, adapter, account, callback_url, callback_key_id, callback_secret, host_id, agent_id, backend, workspace_root, status, capabilities_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			adapter=excluded.adapter,
			account=excluded.account,
			callback_url=excluded.callback_url,
			callback_key_id=excluded.callback_key_id,
			callback_secret=excluded.callback_secret,
			host_id=excluded.host_id,
			agent_id=excluded.agent_id,
			backend=excluded.backend,
			workspace_root=excluded.workspace_root,
			status=excluded.status,
			capabilities_json=excluded.capabilities_json,
			updated_at=excluded.updated_at
	`, normalized.ID, normalized.Adapter, normalized.Account, normalized.CallbackURL, normalized.CallbackKeyID, normalized.CallbackSecret, normalized.Target.HostID, normalized.Target.AgentID, normalized.Target.Backend, normalized.Target.WorkspaceRoot, normalized.Status, string(capsJSON), normalized.CreatedAt, normalized.UpdatedAt); err != nil {
		return integration.Binding{}, fmt.Errorf("upsert integration binding: %w", err)
	}
	return normalized, nil
}

func loadIntegrationBindingByID(bindingID string) (integration.Binding, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()
	return loadIntegrationBindingByIDLocked(bindingID)
}

func issueIntegrationBindingToken(bindingID, issuedBy string) (integration.BindingToken, string, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	binding, err := loadIntegrationBindingByIDLocked(bindingID)
	if err != nil {
		return integration.BindingToken{}, "", err
	}
	if strings.TrimSpace(binding.ID) == "" {
		return integration.BindingToken{}, "", fmt.Errorf("binding %s not found", strings.TrimSpace(bindingID))
	}

	rawToken, err := integration.GenerateTokenRaw(integrationTokenPrefix)
	if err != nil {
		return integration.BindingToken{}, "", err
	}
	record, err := integration.NormalizeBindingToken(integration.BindingToken{
		BindingID:   binding.ID,
		Adapter:     binding.Adapter,
		TokenPrefix: rawToken[:min(len(rawToken), 12)],
		TokenHash:   hashIntegrationToken(rawToken),
		IssuedBy:    strings.TrimSpace(issuedBy),
		Status:      integration.TokenStatusActive,
	})
	if err != nil {
		return integration.BindingToken{}, "", err
	}
	now := nowTimestamp()
	record.CreatedAt = now
	record.UpdatedAt = now

	db, err := openIntegrationDB()
	if err != nil {
		return integration.BindingToken{}, "", err
	}
	defer db.Close()

	if _, err := db.Exec(`
		INSERT INTO integration_binding_tokens (
			id, binding_id, adapter, token_prefix, token_hash, issued_by, status, created_at, updated_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.BindingID, record.Adapter, record.TokenPrefix, record.TokenHash, record.IssuedBy, record.Status, record.CreatedAt, record.UpdatedAt, record.LastUsedAt); err != nil {
		return integration.BindingToken{}, "", fmt.Errorf("insert integration token: %w", err)
	}
	return record, rawToken, nil
}

func authenticateIntegrationToken(rawToken, adapter string) (integration.Binding, integration.BindingToken, bool, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	trimmedToken := strings.TrimSpace(rawToken)
	if trimmedToken == "" {
		return integration.Binding{}, integration.BindingToken{}, false, nil
	}
	normalizedAdapter := strings.TrimSpace(strings.ToLower(adapter))
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Binding{}, integration.BindingToken{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT
			t.id, t.binding_id, t.adapter, t.token_prefix, t.token_hash, t.issued_by, t.status, t.created_at, t.updated_at, t.last_used_at,
			b.id, b.adapter, b.account, b.callback_url, b.callback_key_id, b.callback_secret, b.host_id, b.agent_id, b.backend, b.workspace_root, b.status, b.capabilities_json, b.created_at, b.updated_at
		FROM integration_binding_tokens t
		JOIN integration_bindings b ON b.id = t.binding_id
		WHERE t.token_hash = ? AND t.status = ? AND b.status = ?
	`, hashIntegrationToken(trimmedToken), integration.TokenStatusActive, integration.BindingStatusActive)
	record, binding, found, err := scanIntegrationAuthJoin(row)
	if err != nil {
		return integration.Binding{}, integration.BindingToken{}, false, err
	}
	if !found {
		return integration.Binding{}, integration.BindingToken{}, false, nil
	}
	if normalizedAdapter != "" && !strings.EqualFold(binding.Adapter, normalizedAdapter) {
		return integration.Binding{}, integration.BindingToken{}, false, nil
	}
	record.LastUsedAt = nowTimestamp()
	record.UpdatedAt = record.LastUsedAt
	if _, err := db.Exec(`UPDATE integration_binding_tokens SET last_used_at = ?, updated_at = ? WHERE id = ?`, record.LastUsedAt, record.UpdatedAt, record.ID); err != nil {
		return integration.Binding{}, integration.BindingToken{}, false, fmt.Errorf("update token last_used_at: %w", err)
	}
	return binding, record, true, nil
}

func loadIntegrationBindingByIDLocked(bindingID string) (integration.Binding, error) {
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Binding{}, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT id, adapter, account, callback_url, callback_key_id, callback_secret, host_id, agent_id, backend, workspace_root, status, capabilities_json, created_at, updated_at
		FROM integration_bindings
		WHERE id = ?
	`, strings.TrimSpace(bindingID))
	binding, found, err := scanIntegrationBinding(row)
	if err != nil {
		return integration.Binding{}, err
	}
	if !found {
		return integration.Binding{}, nil
	}
	return binding, nil
}

func openIntegrationDB() (*sql.DB, error) {
	paths, err := integration.ResolvePaths()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(paths.DBPath), 0o700); err != nil {
		return nil, fmt.Errorf("create integration db dir: %w", err)
	}
	db, err := sql.Open("sqlite", paths.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open integration sqlite db: %w", err)
	}
	if err := ensureIntegrationSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureIntegrationSchema(ctx context.Context, db *sql.DB) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS integration_bindings (
			id TEXT PRIMARY KEY,
			adapter TEXT NOT NULL,
			account TEXT NOT NULL,
			callback_url TEXT NOT NULL DEFAULT '',
			callback_key_id TEXT NOT NULL DEFAULT '',
			callback_secret TEXT NOT NULL DEFAULT '',
			host_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			backend TEXT NOT NULL,
			workspace_root TEXT NOT NULL,
			status TEXT NOT NULL,
			capabilities_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS integration_binding_tokens (
			id TEXT PRIMARY KEY,
			binding_id TEXT NOT NULL,
			adapter TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			issued_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(binding_id) REFERENCES integration_bindings(id)
		)`,
		`CREATE TABLE IF NOT EXISTS integration_executions (
			id TEXT PRIMARY KEY,
			binding_id TEXT NOT NULL,
			adapter TEXT NOT NULL,
			account TEXT NOT NULL,
			external_execution_id TEXT NOT NULL DEFAULT '',
			orchestrator_execution_id TEXT NOT NULL DEFAULT '',
			idempotency_key TEXT NOT NULL,
			state TEXT NOT NULL,
			goal TEXT NOT NULL DEFAULT '',
			input TEXT NOT NULL DEFAULT '',
			requested_provider TEXT NOT NULL DEFAULT '',
			failure_category TEXT NOT NULL DEFAULT '',
			failure_code TEXT NOT NULL DEFAULT '',
			current_attempt_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(binding_id) REFERENCES integration_bindings(id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_integration_execution_binding_idempotency ON integration_executions(binding_id, idempotency_key)`,
		`CREATE TABLE IF NOT EXISTS integration_attempts (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			attempt_number INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS integration_actions (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			action_type TEXT NOT NULL,
			idempotency_key TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS integration_events (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL DEFAULT '',
			sequence INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_integration_events_execution_sequence ON integration_events(execution_id, sequence)`,
		`CREATE TABLE IF NOT EXISTS integration_usage_proofs (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			proof_ref TEXT NOT NULL,
			meter_ref TEXT NOT NULL DEFAULT '',
			usage_kind TEXT NOT NULL,
			amount_cents INTEGER NOT NULL DEFAULT 0,
			digest TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS integration_artifact_refs (
			id TEXT PRIMARY KEY,
			execution_id TEXT NOT NULL,
			artifact_ref TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS integration_callback_deliveries (
			id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			execution_id TEXT NOT NULL,
			binding_id TEXT NOT NULL,
			callback_url TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			next_attempt_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(event_id) REFERENCES integration_events(id),
			FOREIGN KEY(execution_id) REFERENCES integration_executions(id),
			FOREIGN KEY(binding_id) REFERENCES integration_bindings(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_integration_callback_deliveries_event ON integration_callback_deliveries(event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_integration_actions_execution_idempotency ON integration_actions(execution_id, idempotency_key)`,
		`CREATE INDEX IF NOT EXISTS idx_integration_token_hash ON integration_binding_tokens(token_hash)`,
	}
	for _, stmt := range ddl {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure integration schema: %w", err)
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE integration_bindings ADD COLUMN callback_key_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE integration_bindings ADD COLUMN callback_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE integration_callback_deliveries ADD COLUMN next_attempt_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("ensure integration schema migration: %w", err)
		}
	}
	return nil
}

func scanIntegrationBinding(scanner interface{ Scan(dest ...any) error }) (integration.Binding, bool, error) {
	var (
		binding integration.Binding
		capsRaw string
	)
	err := scanner.Scan(
		&binding.ID,
		&binding.Adapter,
		&binding.Account,
		&binding.CallbackURL,
		&binding.CallbackKeyID,
		&binding.CallbackSecret,
		&binding.Target.HostID,
		&binding.Target.AgentID,
		&binding.Target.Backend,
		&binding.Target.WorkspaceRoot,
		&binding.Status,
		&capsRaw,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return integration.Binding{}, false, nil
	}
	if err != nil {
		return integration.Binding{}, false, fmt.Errorf("scan integration binding: %w", err)
	}
	if strings.TrimSpace(capsRaw) != "" {
		if err := json.Unmarshal([]byte(capsRaw), &binding.Capabilities); err != nil {
			return integration.Binding{}, false, fmt.Errorf("decode binding capabilities: %w", err)
		}
	}
	return binding, true, nil
}

func scanIntegrationAuthJoin(scanner interface{ Scan(dest ...any) error }) (integration.BindingToken, integration.Binding, bool, error) {
	var (
		record  integration.BindingToken
		binding integration.Binding
		capsRaw string
	)
	err := scanner.Scan(
		&record.ID,
		&record.BindingID,
		&record.Adapter,
		&record.TokenPrefix,
		&record.TokenHash,
		&record.IssuedBy,
		&record.Status,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.LastUsedAt,
		&binding.ID,
		&binding.Adapter,
		&binding.Account,
		&binding.CallbackURL,
		&binding.CallbackKeyID,
		&binding.CallbackSecret,
		&binding.Target.HostID,
		&binding.Target.AgentID,
		&binding.Target.Backend,
		&binding.Target.WorkspaceRoot,
		&binding.Status,
		&capsRaw,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return integration.BindingToken{}, integration.Binding{}, false, nil
	}
	if err != nil {
		return integration.BindingToken{}, integration.Binding{}, false, fmt.Errorf("scan integration auth join: %w", err)
	}
	if strings.TrimSpace(capsRaw) != "" {
		if err := json.Unmarshal([]byte(capsRaw), &binding.Capabilities); err != nil {
			return integration.BindingToken{}, integration.Binding{}, false, fmt.Errorf("decode auth binding capabilities: %w", err)
		}
	}
	return record, binding, true, nil
}

func hashIntegrationToken(rawToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(sum[:])
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
