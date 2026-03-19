package memory

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxDefaultSearchResults = 8
	maxSearchResults        = 50
	defaultSnippetLimit     = 240
	defaultSearchMultiplier = 4
	maxSearchMultiplier     = 20
	maxCandidateLimit       = 300
	minCandidateLimit       = 20
	defaultLexicalWeight    = 0.65
	defaultSemanticWeight   = 0.35
)

var errRecordNotFound = errors.New("memory record not found")

// GetRecord reads a full record if subject has access to its scope.
func (s *Store) GetRecord(subject, id string) (MemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[strings.TrimSpace(id)]
	if !ok {
		return MemoryRecord{}, errRecordNotFound
	}
	allowed := s.allowedScopesForSubjectLocked(strings.TrimSpace(subject))
	if !scopeAllowed(allowed, rec.Scope) {
		return MemoryRecord{}, ErrMountDenied
	}
	return rec, nil
}

// UpsertRecord writes a stable curated record and materializes it to truth files.
func (s *Store) UpsertRecord(input UpsertRecordInput) (MemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scope := normalizeScope(input.Scope)
	subject := strings.TrimSpace(input.Subject)
	if scope == "" {
		scope = Scope("agent:" + subject)
	}
	if isSnapshotScope(scope) {
		return MemoryRecord{}, ErrMountDenied
	}
	allowed := s.allowedWriteScopesForSubjectLocked(subject)
	if !scopeAllowed(allowed, scope) {
		return MemoryRecord{}, ErrMountDenied
	}

	now := s.now()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "rec_" + shortDigest(fmt.Sprintf("%s|%s|%d", scope, input.ContentSummary, now.UnixNano()))
	}
	recordType := input.Type
	if recordType == "" {
		recordType = RecordTypeNote
	}
	contentSummary := strings.TrimSpace(input.ContentSummary)
	if contentSummary == "" {
		contentSummary = clipSnippet(strings.TrimSpace(input.ContentRaw), 500)
	}
	rec, exists := s.records[id]
	if exists {
		if isSnapshotScope(rec.Scope) {
			return MemoryRecord{}, ErrMountDenied
		}
		rec.Scope = scope
		rec.Type = recordType
		rec.ContentRaw = input.ContentRaw
		rec.ContentSummary = contentSummary
		rec.Tags = cloneStrings(input.Tags)
		rec.Provenance = strings.TrimSpace(input.Provenance)
		rec.Confidence = input.Confidence
		rec.Importance = input.Importance
		rec.UpdatedAt = now
		rec.ArchivedAt = nil
	} else {
		rec = MemoryRecord{
			ID:             id,
			Scope:          scope,
			Type:           recordType,
			ContentRaw:     input.ContentRaw,
			ContentSummary: contentSummary,
			Tags:           cloneStrings(input.Tags),
			Provenance:     strings.TrimSpace(input.Provenance),
			Confidence:     input.Confidence,
			Importance:     input.Importance,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	s.records[id] = rec
	s.syncRecordToSQLiteLocked(rec)
	if err := s.writeStableTruthRecordLocked(rec); err != nil {
		return MemoryRecord{}, err
	}
	if err := s.persistStateLocked(); err != nil {
		return MemoryRecord{}, err
	}
	return rec, nil
}

// ArchiveRecord marks a record archived.
func (s *Store) ArchiveRecord(subject, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	rec, ok := s.records[id]
	if !ok {
		return errRecordNotFound
	}
	allowed := s.allowedWriteScopesForSubjectLocked(strings.TrimSpace(subject))
	if !scopeAllowed(allowed, rec.Scope) {
		return ErrMountDenied
	}
	now := s.now()
	rec.ArchivedAt = &now
	rec.UpdatedAt = now
	s.records[id] = rec
	s.syncRecordToSQLiteLocked(rec)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

// PurgeInstanceScope deletes all non-archived records within one writable scope for an instance.
func (s *Store) PurgeInstanceScope(instanceID string, scope Scope) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	instanceID = strings.TrimSpace(instanceID)
	scope = normalizeScope(scope)
	if instanceID == "" {
		return 0, fmt.Errorf("instanceID is required")
	}
	if scope == "" {
		scope = Scope("agent:" + instanceID)
	}
	allowed := s.allowedWriteScopesForSubjectLocked(instanceID)
	if !scopeAllowed(allowed, scope) {
		return 0, ErrMountDenied
	}

	deleted := 0
	for id, rec := range s.records {
		if rec.ArchivedAt != nil {
			continue
		}
		if normalizeScope(rec.Scope) != scope {
			continue
		}
		delete(s.records, id)
		s.deleteRecordFromSQLiteLocked(id)
		deleted++
	}
	if err := s.persistStateLocked(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// Observe stores an append-only event and optionally auto-curates a record.
func (s *Store) Observe(input ObserveInput) (ObservationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subject := strings.TrimSpace(input.Subject)
	scope := normalizeScope(input.Scope)
	if scope == "" {
		if strings.TrimSpace(input.AgentID) != "" {
			scope = Scope("agent:" + strings.TrimSpace(input.AgentID))
		} else {
			scope = Scope("agent:" + subject)
		}
	}
	allowed := s.allowedWriteScopesForSubjectLocked(subject)
	if !scopeAllowed(allowed, scope) {
		return ObservationEvent{}, ErrMountDenied
	}

	now := s.now()
	ev := ObservationEvent{
		ID:            "obs_" + shortDigest(fmt.Sprintf("%s|%s|%d", subject, input.ToolName, now.UnixNano())),
		Timestamp:     now,
		AgentID:       strings.TrimSpace(input.AgentID),
		AppID:         strings.TrimSpace(input.AppID),
		SessionID:     strings.TrimSpace(input.SessionID),
		Scope:         scope,
		ToolName:      strings.TrimSpace(input.ToolName),
		InputsDigest:  strings.TrimSpace(input.InputsDigest),
		OutputSnippet: strings.TrimSpace(input.OutputSnippet),
		Status:        strings.TrimSpace(input.Status),
		Artifacts:     cloneStrings(input.Artifacts),
		Labels:        cloneStrings(input.Labels),
	}
	s.observations = append(s.observations, ev)
	s.gcObservationsLocked()
	s.syncObservationToSQLiteLocked(ev)
	if err := s.appendObservationTruthLocked(ev); err != nil {
		return ObservationEvent{}, err
	}

	if input.AutoCurate && strings.TrimSpace(ev.OutputSnippet) != "" {
		recID := "rec_" + shortDigest(ev.ID)
		s.records[recID] = MemoryRecord{
			ID:             recID,
			Scope:          ev.Scope,
			Type:           RecordTypeNote,
			ContentSummary: clipSnippet(ev.OutputSnippet, 500),
			Provenance:     "observe:" + ev.ID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		s.syncRecordToSQLiteLocked(s.records[recID])
		_ = s.writeStableTruthRecordLocked(s.records[recID])
	}
	if err := s.persistStateLocked(); err != nil {
		return ObservationEvent{}, err
	}
	return ev, nil
}

// Timeline returns nearby events around the pivot event id.
func (s *Store) Timeline(subject, pivotID string, radius int) []ObservationEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if radius <= 0 {
		radius = 3
	}
	if radius > 30 {
		radius = 30
	}
	allowed := s.allowedScopesForSubjectLocked(strings.TrimSpace(subject))
	idx := -1
	for i, ev := range s.observations {
		if ev.ID == strings.TrimSpace(pivotID) && scopeAllowed(allowed, ev.Scope) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius + 1
	if end > len(s.observations) {
		end = len(s.observations)
	}
	out := make([]ObservationEvent, 0, end-start)
	for _, ev := range s.observations[start:end] {
		if scopeAllowed(allowed, ev.Scope) {
			out = append(out, ev)
		}
	}
	return out
}
