package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const sharedSnapshotScopePrefix = "shared:snapshot-"

type snapshotDigestPayload struct {
	SourceSubject string                 `json:"source_subject"`
	SourceScopes  []Scope                `json:"source_scopes"`
	Records       []snapshotDigestRecord `json:"records"`
}

type snapshotDigestRecord struct {
	ID             string     `json:"id"`
	Scope          Scope      `json:"scope"`
	Type           RecordType `json:"type"`
	ContentRaw     string     `json:"content_raw,omitempty"`
	ContentSummary string     `json:"content_summary"`
	Tags           []string   `json:"tags,omitempty"`
	Provenance     string     `json:"provenance,omitempty"`
	Confidence     float64    `json:"confidence,omitempty"`
	Importance     int        `json:"importance,omitempty"`
	UpdatedAt      string     `json:"updated_at,omitempty"`
}

// CreateSnapshotForInstance freezes readable public/shared records into a dedicated read-only scope.
func (s *Store) CreateSnapshotForInstance(ctx context.Context, opts SnapshotOptions) (SnapshotRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SnapshotRecord{}, err
	}

	sourceSubject := strings.TrimSpace(opts.SourceSubject)
	targetInstanceID := strings.TrimSpace(opts.TargetInstanceID)
	reason := strings.TrimSpace(opts.Reason)
	sourceScopes := normalizeAndSortScopes(opts.SourceScopes)
	if sourceSubject == "" || targetInstanceID == "" || len(sourceScopes) == 0 {
		return SnapshotRecord{}, fmt.Errorf("source subject, target instance, and source scopes are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	allowed := s.allowedScopesForSubjectLocked(sourceSubject)
	normalizedScopes := make([]Scope, 0, len(sourceScopes))
	scopeSet := make(map[Scope]struct{}, len(sourceScopes))
	for _, scope := range sourceScopes {
		scope = normalizeScope(scope)
		if !isSnapshotSourceScope(scope) {
			return SnapshotRecord{}, fmt.Errorf("%w: snapshot source scope %q must be public/shared", ErrMountDenied, scope)
		}
		if !scopeAllowed(allowed, scope) {
			return SnapshotRecord{}, ErrMountDenied
		}
		normalizedScopes = append(normalizedScopes, scope)
		scopeSet[scope] = struct{}{}
	}

	sourceRecords := make([]MemoryRecord, 0)
	for _, rec := range s.records {
		if rec.ArchivedAt != nil {
			continue
		}
		if _, ok := scopeSet[normalizeScope(rec.Scope)]; !ok {
			continue
		}
		sourceRecords = append(sourceRecords, rec)
	}
	sort.SliceStable(sourceRecords, func(i, j int) bool {
		if sourceRecords[i].Scope == sourceRecords[j].Scope {
			return sourceRecords[i].ID < sourceRecords[j].ID
		}
		return sourceRecords[i].Scope < sourceRecords[j].Scope
	})

	digest, err := computeSnapshotDigest(sourceSubject, normalizedScopes, sourceRecords)
	if err != nil {
		return SnapshotRecord{}, err
	}
	now := s.now()
	snapshotID := "snap_" + shortDigest(fmt.Sprintf("%s|%s|%s|%d", sourceSubject, targetInstanceID, digest, now.UnixNano()))
	snapshotScope := sharedSnapshotScope(snapshotID)

	sourceRecordIDs := make([]string, 0, len(sourceRecords))
	clonedRecordIDs := make([]string, 0, len(sourceRecords))
	for _, rec := range sourceRecords {
		sourceRecordIDs = append(sourceRecordIDs, rec.ID)
		clone := rec
		clone.ID = "snaprec_" + shortDigest(snapshotID+"|"+string(rec.Scope)+"|"+rec.ID)
		clone.Scope = snapshotScope
		clone.Tags = append(cloneStrings(rec.Tags), "snapshot", "snapshot_id:"+snapshotID)
		if rec.Provenance == "" {
			clone.Provenance = fmt.Sprintf("snapshot:%s:%s", snapshotID, rec.ID)
		} else {
			clone.Provenance = fmt.Sprintf("snapshot:%s:%s|%s", snapshotID, rec.ID, rec.Provenance)
		}
		clone.CreatedAt = now
		clone.UpdatedAt = now
		clone.ArchivedAt = nil
		s.records[clone.ID] = clone
		s.syncRecordToSQLiteLocked(clone)
		clonedRecordIDs = append(clonedRecordIDs, clone.ID)
	}

	snapshot := SnapshotRecord{
		ID:               snapshotID,
		Digest:           digest,
		Scope:            snapshotScope,
		SourceSubject:    sourceSubject,
		SourceScopes:     append([]Scope(nil), normalizedScopes...),
		SourceRecordIDs:  append([]string(nil), sourceRecordIDs...),
		ClonedRecordIDs:  append([]string(nil), clonedRecordIDs...),
		TargetInstanceID: targetInstanceID,
		Reason:           reason,
		CreatedAt:        now,
	}
	if s.snapshots == nil {
		s.snapshots = make(map[string]SnapshotRecord)
	}
	s.snapshots[snapshot.ID] = snapshot
	if err := s.persistStateLocked(); err != nil {
		return SnapshotRecord{}, err
	}
	return snapshot, nil
}

// MountSnapshot exposes a frozen snapshot scope to the target instance as read-only context.
func (s *Store) MountSnapshot(instanceID, snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	instanceID = strings.TrimSpace(instanceID)
	snapshotID = strings.TrimSpace(snapshotID)
	if instanceID == "" || snapshotID == "" {
		return fmt.Errorf("instanceID and snapshotID are required")
	}

	snapshot, ok := s.snapshots[snapshotID]
	if !ok {
		return ErrMemoryNotFound
	}
	if snapshot.TargetInstanceID != "" && snapshot.TargetInstanceID != instanceID {
		return ErrMountDenied
	}

	changed := s.addManualScopeLocked(instanceID, snapshot.Scope)
	if !changed {
		for _, scope := range s.instanceScopes[instanceID] {
			if scope == snapshot.Scope {
				return nil
			}
		}
	}
	s.rebuildInstanceScopesLocked(instanceID)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

// DeleteSnapshot removes snapshot metadata, cloned records, and mounted scope state.
func (s *Store) DeleteSnapshot(snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshotID = strings.TrimSpace(snapshotID)
	snapshot, ok := s.snapshots[snapshotID]
	if !ok {
		return ErrMemoryNotFound
	}

	affectedInstances := make(map[string]struct{})
	for instanceID := range s.manualScopes {
		if s.removeManualScopeLocked(instanceID, snapshot.Scope) {
			affectedInstances[instanceID] = struct{}{}
		}
	}
	for instanceID := range affectedInstances {
		s.rebuildInstanceScopesLocked(instanceID)
	}

	for _, recordID := range snapshot.ClonedRecordIDs {
		delete(s.records, recordID)
		s.deleteRecordFromSQLiteLocked(recordID)
	}
	delete(s.snapshots, snapshotID)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

func sharedSnapshotScope(snapshotID string) Scope {
	return Scope(sharedSnapshotScopePrefix + sanitizeID(strings.TrimSpace(snapshotID)))
}

func isSnapshotScope(scope Scope) bool {
	return strings.HasPrefix(string(normalizeScope(scope)), sharedSnapshotScopePrefix)
}

func isSnapshotSourceScope(scope Scope) bool {
	scope = normalizeScope(scope)
	if scope == ScopePublic {
		return true
	}
	return strings.HasPrefix(string(scope), "shared:") && !isSnapshotScope(scope)
}

func computeSnapshotDigest(sourceSubject string, sourceScopes []Scope, records []MemoryRecord) (string, error) {
	payload := snapshotDigestPayload{
		SourceSubject: strings.TrimSpace(sourceSubject),
		SourceScopes:  append([]Scope(nil), sourceScopes...),
		Records:       make([]snapshotDigestRecord, 0, len(records)),
	}
	for _, rec := range records {
		payload.Records = append(payload.Records, snapshotDigestRecord{
			ID:             rec.ID,
			Scope:          normalizeScope(rec.Scope),
			Type:           rec.Type,
			ContentRaw:     rec.ContentRaw,
			ContentSummary: rec.ContentSummary,
			Tags:           cloneStrings(rec.Tags),
			Provenance:     rec.Provenance,
			Confidence:     rec.Confidence,
			Importance:     rec.Importance,
			UpdatedAt:      rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal snapshot digest payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
