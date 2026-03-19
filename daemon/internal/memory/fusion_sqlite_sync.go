package memory

import (
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func syncRecordTx(tx *sql.Tx, ftsEnabled bool, rec MemoryRecord) error {
	tagsRaw, _ := json.Marshal(rec.Tags)
	var archivedAt interface{}
	if rec.ArchivedAt != nil {
		archivedAt = rec.ArchivedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := tx.Exec(`
		INSERT INTO memory_records(
			id, scope, type, content_raw, content_summary, tags_json, provenance, confidence, importance, created_at, updated_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			type=excluded.type,
			content_raw=excluded.content_raw,
			content_summary=excluded.content_summary,
			tags_json=excluded.tags_json,
			provenance=excluded.provenance,
			confidence=excluded.confidence,
			importance=excluded.importance,
			created_at=excluded.created_at,
			updated_at=excluded.updated_at,
			archived_at=excluded.archived_at
	`,
		rec.ID,
		string(rec.Scope),
		string(rec.Type),
		rec.ContentRaw,
		rec.ContentSummary,
		string(tagsRaw),
		rec.Provenance,
		rec.Confidence,
		rec.Importance,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano),
		rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		archivedAt,
	); err != nil {
		return err
	}
	if ftsEnabled {
		if _, err := tx.Exec(`DELETE FROM records_fts WHERE id = ?`, rec.ID); err != nil {
			return err
		}
		if rec.ArchivedAt == nil {
			if _, err := tx.Exec(
				`INSERT INTO records_fts(text, id, scope, provenance, tags) VALUES(?,?,?,?,?)`,
				strings.TrimSpace(rec.ContentSummary+"\n"+rec.ContentRaw),
				rec.ID,
				string(rec.Scope),
				rec.Provenance,
				string(tagsRaw),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func syncObservationExec(exec execer, ev ObservationEvent) error {
	return syncObservationExecAt(exec, ev, false)
}

func syncObservationTx(tx *sql.Tx, ev ObservationEvent) error {
	return syncObservationExecAt(tx, ev, true)
}

func syncObservationExecAt(exec execer, ev ObservationEvent, upsert bool) error {
	artifactsRaw, _ := json.Marshal(ev.Artifacts)
	labelsRaw, _ := json.Marshal(ev.Labels)
	stmt := `
		INSERT INTO observation_events(
			id, ts, agent_id, app_id, session_id, scope, tool_name, inputs_digest, output_snippet, status, artifacts_json, labels_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if upsert {
		stmt += `
		ON CONFLICT(id) DO UPDATE SET
			ts=excluded.ts,
			agent_id=excluded.agent_id,
			app_id=excluded.app_id,
			session_id=excluded.session_id,
			scope=excluded.scope,
			tool_name=excluded.tool_name,
			inputs_digest=excluded.inputs_digest,
			output_snippet=excluded.output_snippet,
			status=excluded.status,
			artifacts_json=excluded.artifacts_json,
			labels_json=excluded.labels_json`
	}
	_, err := exec.Exec(
		stmt,
		ev.ID,
		ev.Timestamp.UTC().Format(time.RFC3339Nano),
		ev.AgentID,
		ev.AppID,
		ev.SessionID,
		string(ev.Scope),
		ev.ToolName,
		ev.InputsDigest,
		ev.OutputSnippet,
		ev.Status,
		string(artifactsRaw),
		string(labelsRaw),
	)
	return err
}

func syncGrantExec(exec execer, g Grant) error {
	return syncGrantExecAt(exec, g, false)
}

func syncGrantTx(tx *sql.Tx, g Grant) error {
	return syncGrantExecAt(tx, g, true)
}

func syncGrantExecAt(exec execer, g Grant, upsert bool) error {
	var revokedAt interface{}
	if g.RevokedAt != nil {
		revokedAt = g.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	stmt := `
		INSERT INTO grants(
			id, subject, scope, granted_by, granted_at, reason, revoked_by, revoked_at
		) VALUES(?,?,?,?,?,?,?,?)
	`
	if upsert {
		stmt += `
		ON CONFLICT(id) DO UPDATE SET
			subject=excluded.subject,
			scope=excluded.scope,
			granted_by=excluded.granted_by,
			granted_at=excluded.granted_at,
			reason=excluded.reason,
			revoked_by=excluded.revoked_by,
			revoked_at=excluded.revoked_at`
	}
	_, err := exec.Exec(
		stmt,
		g.ID,
		g.Subject,
		string(g.Scope),
		g.GrantedBy,
		g.GrantedAt.UTC().Format(time.RFC3339Nano),
		g.Reason,
		g.RevokedBy,
		revokedAt,
	)
	return err
}

func buildFTSQuery(query string) string {
	parts := strings.Fields(strings.TrimSpace(query))
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		if part == "" {
			continue
		}
		if strings.ContainsAny(part, `*^()[]{}:"`) {
			part = strconv.Quote(part)
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return query
	}
	return strings.Join(out, " AND ")
}

func rankToScore(rank float64) float64 {
	if math.IsNaN(rank) {
		return 0
	}
	if rank < 0 {
		rank = 0
	}
	return 1 / (1 + rank)
}
