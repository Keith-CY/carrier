package memory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
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

type searchCandidate struct {
	hit      SearchHit
	lexical  float64
	semantic float64
}

var errRecordNotFound = errors.New("memory record not found")

// Search performs a compact record recall with permission-first filtering.
func (s *Store) Search(opts SearchOptions) []SearchHit {
	s.mu.RLock()

	subject := strings.TrimSpace(opts.Subject)
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		s.mu.RUnlock()
		return nil
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = maxDefaultSearchResults
	}
	if maxResults > maxSearchResults {
		maxResults = maxSearchResults
	}
	allowed := s.allowedScopesForSubjectLocked(subject)
	includeDistilled := optionBool(opts.IncludeDistilled, true)
	includeRaw := optionBool(opts.IncludeRaw, true)

	candidateMultiplier := opts.CandidateMultiplier
	if candidateMultiplier <= 0 {
		candidateMultiplier = defaultSearchMultiplier
	}
	if candidateMultiplier > maxSearchMultiplier {
		candidateMultiplier = maxSearchMultiplier
	}
	candidateLimit := maxResults * candidateMultiplier
	if candidateLimit < minCandidateLimit {
		candidateLimit = minCandidateLimit
	}
	if candidateLimit > maxCandidateLimit {
		candidateLimit = maxCandidateLimit
	}

	lexicalWeight := optionFloat(opts.LexicalWeight, defaultLexicalWeight)
	semanticWeight := optionFloat(opts.SemanticWeight, defaultSemanticWeight)
	enableRerank := optionBool(opts.Rerank, true)
	enableAdaptiveRecall := optionBool(opts.AdaptiveRecall, true)
	if lexicalWeight < 0 {
		lexicalWeight = 0
	}
	if semanticWeight < 0 {
		semanticWeight = 0
	}
	weightSum := lexicalWeight + semanticWeight
	if weightSum == 0 {
		lexicalWeight = defaultLexicalWeight
		semanticWeight = defaultSemanticWeight
		weightSum = lexicalWeight + semanticWeight
	}
	lexicalWeight /= weightSum
	semanticWeight /= weightSum

	candidates := make(map[string]searchCandidate, maxResults)
	usedSQLite := false
	sqliteMinScore := opts.MinScore
	if enableRerank {
		sqliteMinScore = 0
	}
	lowerQuery := strings.ToLower(query)
	if sqliteHits, ok := s.searchSQLiteLocked(allowed, query, candidateLimit, sqliteMinScore); ok {
		for _, hit := range sqliteHits {
			candidates[hit.ID] = searchCandidate{
				hit:     hit,
				lexical: hit.Score,
			}
		}
		usedSQLite = true
	}
	needInMemory := !usedSQLite || (enableAdaptiveRecall && len(candidates) < maxResults)
	if needInMemory {
		inMemoryHits := s.searchInMemoryLocked(allowed, query, candidateLimit, 0)
		for _, hit := range inMemoryHits {
			existing, ok := candidates[hit.ID]
			if !ok || hit.Score > existing.lexical {
				candidates[hit.ID] = searchCandidate{
					hit:     hit,
					lexical: hit.Score,
				}
			}
		}
	}

	results := make([]SearchHit, 0, len(candidates))
	for _, candidate := range candidates {
		contentText := candidate.hit.Snippet
		isDistilled := false
		if rec, ok := s.records[candidate.hit.ID]; ok {
			isDistilled = isDistilledRecord(rec)
			if isDistilled && !includeDistilled {
				continue
			}
			if !isDistilled && !includeRaw {
				continue
			}
			contentText = strings.TrimSpace(rec.ContentSummary)
			if contentText == "" {
				contentText = strings.TrimSpace(rec.ContentRaw)
			}
		} else if !includeRaw {
			continue
		}
		semanticScore := scoreText(contentText, lowerQuery)
		candidate.semantic = semanticScore
		if enableRerank {
			candidate.hit.Score = blendScore(candidate.lexical, candidate.semantic, lexicalWeight, semanticWeight)
		} else {
			candidate.hit.Score = candidate.lexical
		}
		candidate.hit.Score *= distilledScoreMultiplier(query, isDistilled)
		if opts.MinScore > 0 && candidate.hit.Score < opts.MinScore {
			continue
		}
		results = append(results, candidate.hit)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ID < results[j].ID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	resultIDs := make([]string, 0, len(results))
	for _, hit := range results {
		resultIDs = append(resultIDs, hit.ID)
	}
	s.mu.RUnlock()
	s.recordSearchHits(resultIDs)
	return results
}

func (s *Store) searchInMemoryLocked(allowed map[Scope]struct{}, query string, maxResults int, minScore float64) []SearchHit {
	hits := make([]SearchHit, 0, maxResults)
	lowerQuery := strings.ToLower(query)
	for _, rec := range s.records {
		if rec.ArchivedAt != nil {
			continue
		}
		if !scopeAllowed(allowed, rec.Scope) {
			continue
		}
		text := strings.TrimSpace(rec.ContentSummary)
		if text == "" {
			text = strings.TrimSpace(rec.ContentRaw)
		}
		if text == "" {
			continue
		}
		score := scoreText(text, lowerQuery)
		if score <= 0 {
			continue
		}
		if minScore > 0 && score < minScore {
			continue
		}
		hits = append(hits, SearchHit{
			ID:         rec.ID,
			Scope:      rec.Scope,
			Score:      score,
			Snippet:    clipSnippet(text, defaultSnippetLimit),
			Provenance: rec.Provenance,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits
}

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

// GrantScope adds explicit scope authorization for a subject.
func (s *Store) GrantScope(subject string, scope Scope, grantedBy, reason string) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subject = strings.TrimSpace(subject)
	scope = normalizeScope(scope)
	if subject == "" || scope == "" {
		return Grant{}, fmt.Errorf("subject and scope are required")
	}
	if isSnapshotScope(scope) {
		return Grant{}, ErrMountDenied
	}
	now := s.now()
	id := "grant_" + shortDigest(subject+"|"+string(scope)+"|"+fmt.Sprintf("%d", now.UnixNano()))
	g := Grant{
		ID:        id,
		Subject:   subject,
		Scope:     scope,
		GrantedBy: strings.TrimSpace(grantedBy),
		GrantedAt: now,
		Reason:    strings.TrimSpace(reason),
	}
	s.grants[id] = g
	s.syncGrantToSQLiteLocked(g)
	if err := s.persistStateLocked(); err != nil {
		return Grant{}, err
	}
	return g, nil
}

// RevokeScope revokes existing authorization immediately.
func (s *Store) RevokeScope(grantID, revokedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grantID = strings.TrimSpace(grantID)
	g, ok := s.grants[grantID]
	if !ok {
		return ErrMemoryNotFound
	}
	now := s.now()
	g.RevokedAt = &now
	g.RevokedBy = strings.TrimSpace(revokedBy)
	s.grants[grantID] = g
	s.syncGrantToSQLiteLocked(g)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

// ListGrants returns all grants for a subject.
func (s *Store) ListGrants(subject string) []Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subject = strings.TrimSpace(subject)
	out := make([]Grant, 0)
	for _, g := range s.grants {
		if subject == "" || g.Subject == subject {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GrantedAt.Before(out[j].GrantedAt) })
	return out
}

// AttachScope adds a scope to an agent instance's mounted memory view.
func (s *Store) AttachScope(instanceID string, scope Scope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	instanceID = strings.TrimSpace(instanceID)
	scope = normalizeScope(scope)
	if instanceID == "" || scope == "" {
		return fmt.Errorf("instanceID and scope are required")
	}
	if isSnapshotScope(scope) {
		return ErrMountDenied
	}
	changed := s.addManualScopeLocked(instanceID, scope)
	if !changed {
		existing := s.instanceScopes[instanceID]
		for _, v := range existing {
			if v == scope {
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

// DetachScope removes a scope from an instance view without deleting data.
func (s *Store) DetachScope(instanceID string, scope Scope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	instanceID = strings.TrimSpace(instanceID)
	scope = normalizeScope(scope)
	existing := s.instanceScopes[instanceID]
	idx := -1
	for i, v := range existing {
		if v == scope {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrAttachmentMissing
	}
	_ = s.removeManualScopeLocked(instanceID, scope)
	updated := append(existing[:idx], existing[idx+1:]...)
	updated = normalizeAndSortScopes(updated)
	if len(updated) == 0 {
		delete(s.instanceScopes, instanceID)
		s.syncInstanceScopesToSQLiteLocked(instanceID, nil)
	} else {
		s.instanceScopes[instanceID] = updated
		s.syncInstanceScopesToSQLiteLocked(instanceID, updated)
	}
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

// InstanceScopes returns mounted scopes for one instance.
func (s *Store) InstanceScopes(instanceID string) []Scope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	scopes := s.instanceScopes[strings.TrimSpace(instanceID)]
	out := make([]Scope, len(scopes))
	copy(out, scopes)
	return out
}

// ImportForInstance imports truth markdown or legacy mempack for one instance.
func (s *Store) ImportForInstance(instanceID, sourcePath string, opts InstanceImportOptions) (string, error) {
	instanceID = strings.TrimSpace(instanceID)
	sourcePath = strings.TrimSpace(sourcePath)
	if instanceID == "" || sourcePath == "" {
		return "", fmt.Errorf("instanceID and sourcePath are required")
	}
	if strings.HasSuffix(strings.ToLower(sourcePath), ".mempack.zip") {
		entry, err := s.ImportMemory(sourcePath, ImportOptions{
			TargetRegion: TypePerAgent,
			Owner:        instanceID,
			Actor:        opts.Actor,
			RequestID:    opts.RequestID,
		})
		if err != nil {
			return "", err
		}
		_ = s.AttachScope(instanceID, scopeForEntry(entry))
		return entry.ID, nil
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read import source: %w", err)
	}
	scope := normalizeScope(opts.TargetScope)
	if scope == "" {
		scope = Scope("agent:" + instanceID)
	}
	rec, err := s.UpsertRecord(UpsertRecordInput{
		Subject:        instanceID,
		Scope:          scope,
		Type:           RecordTypeNote,
		ContentRaw:     string(raw),
		ContentSummary: clipSnippet(string(raw), 500),
		Provenance:     "import:" + sourcePath,
	})
	if err != nil {
		return "", err
	}
	return rec.ID, nil
}

// ExportForInstance exports one instance's memory view.
func (s *Store) ExportForInstance(instanceID string, opts InstanceExportOptions) (string, error) {
	root, err := s.requireRootDir()
	if err != nil {
		return "", err
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return "", fmt.Errorf("instanceID is required")
	}
	format := strings.TrimSpace(strings.ToLower(opts.Format))
	if format == "" {
		format = "truth-only"
	}

	truthPath := filepath.Join(s.truthRoot, "agent", instanceID)
	if _, statErr := os.Stat(truthPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", ErrMemoryNotFound
		}
		return "", statErr
	}

	exportsDir := filepath.Join(root, "artifacts", "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s.fusionmem.zip", sanitizeID(instanceID))
	outPath := filepath.Join(exportsDir, filename)
	file, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	if err := addPathToZip(zipWriter, truthPath, filepath.Join("truth", "agent", instanceID)); err != nil {
		return "", err
	}
	if format == "truth+index" && strings.TrimSpace(s.indexPath) != "" {
		if _, statErr := os.Stat(s.indexPath); statErr == nil {
			if err := addPathToZip(zipWriter, s.indexPath, filepath.Join("index", filepath.Base(s.indexPath))); err != nil {
				return "", err
			}
		}
	}
	if err := zipWriter.Close(); err != nil {
		return "", err
	}
	s.recordAudit(opts.RequestID, opts.Actor, "instance_export", instanceID, auditResultSuccess, "instance exported")
	return outPath, nil
}

func (s *Store) migrateLegacyToFusionLocked() {
	for _, entry := range s.entries {
		scope := scopeForEntry(entry)
		if scope == "" {
			continue
		}
		if _, exists := s.records[entry.ID]; exists {
			continue
		}
		rec := MemoryRecord{
			ID:             entry.ID,
			Scope:          scope,
			Type:           RecordTypeNote,
			ContentSummary: strings.TrimSpace(entry.Name),
			Provenance:     "legacy:entry",
			CreatedAt:      entry.CreatedAt,
			UpdatedAt:      entry.UpdatedAt,
		}
		if entry.State == StateArchived {
			archived := entry.UpdatedAt
			rec.ArchivedAt = &archived
		}
		s.records[rec.ID] = rec
	}
	for instanceID, links := range s.attachments {
		existing := s.instanceScopes[instanceID]
		seen := make(map[Scope]struct{}, len(existing))
		for _, sc := range existing {
			seen[sc] = struct{}{}
		}
		for _, memID := range links {
			entry, ok := s.entries[memID.MemoryID]
			if !ok {
				continue
			}
			scope := scopeForEntry(entry)
			if scope == "" {
				continue
			}
			if _, ok := seen[scope]; ok {
				continue
			}
			existing = append(existing, scope)
			seen[scope] = struct{}{}
		}
		sort.SliceStable(existing, func(i, j int) bool { return existing[i] < existing[j] })
		s.instanceScopes[instanceID] = existing
	}
}

func (s *Store) gcObservationsLocked() {
	if s.retentionDays <= 0 {
		return
	}
	cutoff := s.now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	filtered := s.observations[:0]
	for _, ev := range s.observations {
		if ev.Timestamp.IsZero() || ev.Timestamp.After(cutoff) {
			filtered = append(filtered, ev)
		}
	}
	s.observations = filtered
	s.lastObservationGC = s.now()
}

func (s *Store) allowedScopesForSubjectLocked(subject string) map[Scope]struct{} {
	allowed := map[Scope]struct{}{
		ScopePublic: {},
	}
	subject = strings.TrimSpace(subject)
	if subject != "" {
		allowed[Scope("agent:"+subject)] = struct{}{}
	}
	for _, sc := range s.instanceScopes[subject] {
		scope := normalizeScope(sc)
		if isSnapshotScope(scope) {
			snapshot, ok := s.snapshotForScopeLocked(scope)
			if !ok || strings.TrimSpace(snapshot.TargetInstanceID) != subject {
				continue
			}
		}
		allowed[scope] = struct{}{}
	}
	for _, g := range s.grants {
		if g.Subject != subject {
			continue
		}
		if g.RevokedAt != nil {
			continue
		}
		scope := normalizeScope(g.Scope)
		if isSnapshotScope(scope) {
			continue
		}
		allowed[scope] = struct{}{}
	}
	return allowed
}

func (s *Store) allowedWriteScopesForSubjectLocked(subject string) map[Scope]struct{} {
	allowed := map[Scope]struct{}{}
	subject = strings.TrimSpace(subject)
	if subject != "" {
		allowed[Scope("agent:"+subject)] = struct{}{}
	}
	for _, g := range s.grants {
		if g.Subject != subject {
			continue
		}
		if g.RevokedAt != nil {
			continue
		}
		scope := normalizeScope(g.Scope)
		if isSnapshotScope(scope) {
			continue
		}
		if strings.HasPrefix(string(scope), "agent:") || strings.HasPrefix(string(scope), "shared:") {
			allowed[scope] = struct{}{}
		}
	}
	return allowed
}

func scopeAllowed(allowed map[Scope]struct{}, candidate Scope) bool {
	candidate = normalizeScope(candidate)
	if candidate == "" {
		return false
	}
	if _, ok := allowed[candidate]; ok {
		return true
	}
	if isSnapshotScope(candidate) {
		return false
	}
	for granted := range allowed {
		gs := string(granted)
		if strings.HasSuffix(gs, "*") {
			prefix := strings.TrimSuffix(gs, "*")
			if strings.HasPrefix(string(candidate), prefix) {
				return true
			}
		}
	}
	return false
}

func scoreText(text, lowerQuery string) float64 {
	lower := strings.ToLower(text)
	lower = strings.TrimSpace(lower)
	if lowerQuery == "" || lower == "" {
		return 0
	}
	parts := tokenizeText(lowerQuery)
	if len(parts) == 0 {
		return 0
	}
	matched := 0.0
	textParts := tokenizeText(lower)
	if len(textParts) == 0 {
		return 0
	}
	textSet := make(map[string]struct{}, len(textParts))
	for _, part := range textParts {
		if part == "" {
			continue
		}
		textSet[part] = struct{}{}
	}
	for _, part := range parts {
		if _, ok := textSet[part]; ok {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	ratio := matched / float64(len(parts))
	return ratio
}

func blendScore(lexicalScore, semanticScore, lexicalWeight, semanticWeight float64) float64 {
	if lexicalWeight+semanticWeight == 0 {
		return lexicalScore
	}
	sum := lexicalWeight + semanticWeight
	return (lexicalScore*lexicalWeight + semanticScore*semanticWeight) / sum
}

func optionBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func optionFloat(v *float64, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return *v
}

func tokenizeText(input string) []string {
	input = strings.ToLower(input)
	return strings.FieldsFunc(input, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

func clipSnippet(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func normalizeScope(scope Scope) Scope {
	raw := strings.TrimSpace(string(scope))
	if raw == "" {
		return ""
	}
	if raw == "public" {
		return ScopePublic
	}
	return Scope(raw)
}

func scopeForEntry(entry Entry) Scope {
	switch entry.Type {
	case TypePerAgent:
		owner := strings.TrimSpace(entry.Owner)
		if owner == "" {
			return ""
		}
		return Scope("agent:" + owner)
	case TypeShared:
		return Scope("shared:default")
	case TypePublic:
		return ScopePublic
	default:
		return ""
	}
}

func (s *Store) appendObservationTruthLocked(ev ObservationEvent) error {
	root := strings.TrimSpace(s.truthRoot)
	if root == "" {
		return nil
	}
	var path string
	switch {
	case strings.HasPrefix(string(ev.Scope), "agent:"):
		agentID := strings.TrimPrefix(string(ev.Scope), "agent:")
		path = filepath.Join(root, "agent", agentID, "daily", s.now().UTC().Format("2006-01-02")+".md")
	case strings.HasPrefix(string(ev.Scope), "shared:"):
		ns := strings.TrimPrefix(string(ev.Scope), "shared:")
		path = filepath.Join(root, "shared", ns, "daily", s.now().UTC().Format("2006-01-02")+".md")
	default:
		path = filepath.Join(root, "public", "daily", s.now().UTC().Format("2006-01-02")+".md")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line := fmt.Sprintf("- [%s] (%s) %s\n", ev.Timestamp.UTC().Format(time.RFC3339), ev.ToolName, clipSnippet(ev.OutputSnippet, 800))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, line)
	return err
}

func (s *Store) writeStableTruthRecordLocked(rec MemoryRecord) error {
	root := strings.TrimSpace(s.truthRoot)
	if root == "" {
		return nil
	}
	scope := normalizeScope(rec.Scope)
	var path string
	switch {
	case scope == ScopePublic:
		path = filepath.Join(root, "public", "MEMORY.md")
	case strings.HasPrefix(string(scope), "shared:"):
		ns := strings.TrimPrefix(string(scope), "shared:")
		path = filepath.Join(root, "shared", ns, "MEMORY.md")
	case strings.HasPrefix(string(scope), "agent:"):
		agentID := strings.TrimPrefix(string(scope), "agent:")
		path = filepath.Join(root, "agent", agentID, "MEMORY.md")
	default:
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	block := fmt.Sprintf("\n## [%s] %s\n- updated_at: %s\n- provenance: %s\n\n%s\n",
		rec.ID, rec.Type, rec.UpdatedAt.UTC().Format(time.RFC3339), rec.Provenance, strings.TrimSpace(rec.ContentSummary))
	_, err = io.WriteString(f, block)
	return err
}

func shortDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:12]
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func addPathToZip(zw *zip.Writer, absPath string, archivePath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return addFileToZip(zw, absPath, archivePath)
	}
	entries, err := listFiles(absPath)
	if err != nil {
		return err
	}
	for _, file := range entries {
		rel, relErr := filepath.Rel(absPath, file)
		if relErr != nil {
			return relErr
		}
		target := filepath.ToSlash(filepath.Join(archivePath, rel))
		if err := addFileToZip(zw, file, target); err != nil {
			return err
		}
	}
	return nil
}

func addFileToZip(zw *zip.Writer, absPath, archivePath string) error {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	w, err := zw.Create(filepath.ToSlash(archivePath))
	if err != nil {
		return err
	}
	_, err = w.Write(raw)
	return err
}
