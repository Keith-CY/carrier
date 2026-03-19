package memory

import (
	"sort"
	"strings"
	"time"
)

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
