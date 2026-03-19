package memory

import (
	"fmt"
	"sort"
	"strings"
)

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
