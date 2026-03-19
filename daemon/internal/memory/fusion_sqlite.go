package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

func (s *Store) rebuildSQLiteIndexLocked() {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM observation_events`); err != nil {
		s.lastStateErr = err
		return
	}
	if _, err := tx.Exec(`DELETE FROM grants`); err != nil {
		s.lastStateErr = err
		return
	}
	if _, err := tx.Exec(`DELETE FROM instance_mounts`); err != nil {
		s.lastStateErr = err
		return
	}
	if _, err := tx.Exec(`DELETE FROM memory_records`); err != nil {
		s.lastStateErr = err
		return
	}
	if s.sqliteFTSEnabled {
		if _, err := tx.Exec(`DELETE FROM records_fts`); err != nil {
			s.lastStateErr = err
			return
		}
	}

	for _, rec := range s.records {
		if err := syncRecordTx(tx, s.sqliteFTSEnabled, rec); err != nil {
			s.lastStateErr = err
			return
		}
	}
	for _, ev := range s.observations {
		if err := syncObservationTx(tx, ev); err != nil {
			s.lastStateErr = err
			return
		}
	}
	for _, g := range s.grants {
		if err := syncGrantTx(tx, g); err != nil {
			s.lastStateErr = err
			return
		}
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for instanceID, scopes := range s.instanceScopes {
		for _, scope := range scopes {
			if _, err := tx.Exec(
				`INSERT INTO instance_mounts(instance_id, scope, attached_at) VALUES(?,?,?)`,
				instanceID, string(scope), now,
			); err != nil {
				s.lastStateErr = err
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		s.lastStateErr = err
		return
	}
}

func (s *Store) ensureSQLiteIndexLocked() bool {
	if strings.TrimSpace(s.indexPath) == "" {
		return false
	}
	if s.sqliteReady {
		return true
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return false
	}
	defer db.Close()

	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS memory_records (
			id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			type TEXT NOT NULL,
			content_raw TEXT NOT NULL DEFAULT '',
			content_summary TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			provenance TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0,
			importance INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			archived_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_records_scope ON memory_records(scope);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_records_updated ON memory_records(updated_at);`,
		`CREATE TABLE IF NOT EXISTS observation_events (
			id TEXT PRIMARY KEY,
			ts TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			app_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL,
			tool_name TEXT NOT NULL DEFAULT '',
			inputs_digest TEXT NOT NULL DEFAULT '',
			output_snippet TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			artifacts_json TEXT NOT NULL DEFAULT '[]',
			labels_json TEXT NOT NULL DEFAULT '[]'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_observation_scope_ts ON observation_events(scope, ts);`,
		`CREATE TABLE IF NOT EXISTS grants (
			id TEXT PRIMARY KEY,
			subject TEXT NOT NULL,
			scope TEXT NOT NULL,
			granted_by TEXT NOT NULL DEFAULT '',
			granted_at TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			revoked_by TEXT NOT NULL DEFAULT '',
			revoked_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_grants_subject ON grants(subject);`,
		`CREATE TABLE IF NOT EXISTS instance_mounts (
			instance_id TEXT NOT NULL,
			scope TEXT NOT NULL,
			attached_at TEXT NOT NULL,
			PRIMARY KEY(instance_id, scope)
		);`,
		`CREATE TABLE IF NOT EXISTS memory_distill_manifests (
			run_id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			instance_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			source_batch_json TEXT NOT NULL,
			checksum TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			s.lastStateErr = fmt.Errorf("init sqlite index: %w", err)
			return false
		}
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version','fusionmem_v1')`); err != nil {
		s.lastStateErr = err
		return false
	}

	s.sqliteFTSEnabled = true
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS records_fts USING fts5(
		text,
		id UNINDEXED,
		scope UNINDEXED,
		provenance UNINDEXED,
		tags UNINDEXED
	);`); err != nil {
		// Keep working without FTS.
		s.sqliteFTSEnabled = false
	}
	s.sqliteReady = true
	return true
}

func (s *Store) openSQLiteLocked() (*sql.DB, error) {
	indexPath := strings.TrimSpace(s.indexPath)
	if indexPath == "" {
		return nil, fmt.Errorf("sqlite index path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite index dir: %w", err)
	}
	dsn := indexPath + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) syncRecordToSQLiteLocked(rec MemoryRecord) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer func() { _ = tx.Rollback() }()
	if err := syncRecordTx(tx, s.sqliteFTSEnabled, rec); err != nil {
		s.lastStateErr = err
		return
	}
	if err := tx.Commit(); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) syncObservationToSQLiteLocked(ev ObservationEvent) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()
	if err := syncObservationExec(db, ev); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) syncGrantToSQLiteLocked(g Grant) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()
	if err := syncGrantExec(db, g); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) syncInstanceScopesToSQLiteLocked(instanceID string, scopes []Scope) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM instance_mounts WHERE instance_id = ?`, instanceID); err != nil {
		s.lastStateErr = err
		return
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, scope := range scopes {
		if _, err := tx.Exec(
			`INSERT INTO instance_mounts(instance_id, scope, attached_at) VALUES(?,?,?)`,
			instanceID, string(scope), now,
		); err != nil {
			s.lastStateErr = err
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) deleteRecordFromSQLiteLocked(recordID string) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM records_fts WHERE id = ?`, recordID); err != nil {
		s.lastStateErr = err
		return
	}
	if _, err := tx.Exec(`DELETE FROM memory_records WHERE id = ?`, recordID); err != nil {
		s.lastStateErr = err
		return
	}
	if err := tx.Commit(); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) syncDistillManifestToSQLiteLocked(m DistillSourceManifest) {
	if !s.ensureSQLiteIndexLocked() {
		return
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return
	}
	defer db.Close()

	raw, err := json.Marshal(m)
	if err != nil {
		s.lastStateErr = err
		return
	}
	checksum := shortDigest(string(raw))
	if _, err := db.Exec(`
		INSERT INTO memory_distill_manifests(run_id, scope, instance_id, created_at, source_batch_json, checksum)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(run_id) DO UPDATE SET
			scope=excluded.scope,
			instance_id=excluded.instance_id,
			created_at=excluded.created_at,
			source_batch_json=excluded.source_batch_json,
			checksum=excluded.checksum
	`, m.RunID, string(m.Scope), m.InstanceID, m.CreatedAt.UTC().Format(time.RFC3339Nano), string(raw), checksum); err != nil {
		s.lastStateErr = err
	}
}

func (s *Store) searchSQLiteLocked(allowed map[Scope]struct{}, query string, maxResults int, minScore float64) ([]SearchHit, bool) {
	if !s.ensureSQLiteIndexLocked() || !s.sqliteFTSEnabled {
		return nil, false
	}
	db, err := s.openSQLiteLocked()
	if err != nil {
		s.lastStateErr = err
		return nil, false
	}
	defer db.Close()

	searchLimit := maxResults
	if searchLimit < 20 {
		searchLimit = 20
	}
	ftsQuery := buildFTSQuery(query)
	rows, err := db.Query(`
		SELECT r.id, r.scope, r.provenance, r.content_summary, bm25(records_fts) AS rank
		FROM records_fts
		JOIN memory_records r ON r.id = records_fts.id
		WHERE records_fts MATCH ? AND r.archived_at IS NULL
		ORDER BY rank ASC
		LIMIT ?;
	`, ftsQuery, searchLimit)
	if err != nil {
		s.lastStateErr = err
		return nil, false
	}
	defer rows.Close()

	out := make([]SearchHit, 0, maxResults)
	for rows.Next() {
		var (
			id         string
			scopeRaw   string
			provenance string
			summary    string
			rank       float64
		)
		if err := rows.Scan(&id, &scopeRaw, &provenance, &summary, &rank); err != nil {
			s.lastStateErr = err
			return nil, false
		}
		scope := normalizeScope(Scope(scopeRaw))
		if !scopeAllowed(allowed, scope) {
			continue
		}
		score := rankToScore(rank)
		if minScore > 0 && score < minScore {
			continue
		}
		out = append(out, SearchHit{
			ID:         id,
			Scope:      scope,
			Score:      score,
			Snippet:    clipSnippet(summary, defaultSnippetLimit),
			Provenance: provenance,
		})
		if len(out) >= maxResults {
			break
		}
	}
	if err := rows.Err(); err != nil {
		s.lastStateErr = err
		return nil, false
	}
	return out, true
}
