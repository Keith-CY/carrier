package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AttachMemory binds a memory package to an agent with explicit mode and priority.
func (s *Store) AttachMemory(agentID, memoryID string, opts AttachOptions) (Attachment, error) {
	s.mu.Lock()
	att, auditMessage, err := s.attachMemoryLocked(agentID, memoryID, opts)
	s.mu.Unlock()

	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "attach", memoryID, auditResultFailure, auditMessage)
		return Attachment{}, err
	}
	s.recordAudit(opts.RequestID, opts.Actor, "attach", memoryID, auditResultSuccess, auditMessage)
	return att, nil
}

func (s *Store) attachMemoryLocked(agentID, memoryID string, opts AttachOptions) (Attachment, string, error) {
	entry, ok := s.entries[memoryID]
	if !ok {
		return Attachment{}, ErrMemoryNotFound.Error(), ErrMemoryNotFound
	}
	if entry.State == StateArchived {
		err := fmt.Errorf("cannot attach archived memory %s", memoryID)
		return Attachment{}, err.Error(), err
	}

	existing := s.attachments[agentID]
	for _, a := range existing {
		if a.MemoryID == memoryID {
			return Attachment{}, ErrAlreadyMounted.Error(), ErrAlreadyMounted
		}
		if entry.Type == TypePerAgent && a.MemoryID != memoryID {
			if e, ok := s.entries[a.MemoryID]; ok && e.Type == TypePerAgent {
				return Attachment{}, ErrPerAgentLimit.Error(), ErrPerAgentLimit
			}
		}
	}

	if entry.Type == TypePerAgent && entry.Owner != "" && entry.Owner != agentID {
		err := fmt.Errorf("%w: memory owner is %q, requester is %q", ErrOwnerMismatch, entry.Owner, agentID)
		return Attachment{}, err.Error(), err
	}

	effectiveMode := s.policy.DefaultAccessMode(entry.Type)
	if opts.Mode == AccessReadOnly || opts.Mode == AccessReadWrite {
		effectiveMode = s.policy.ResolveAccessMode(entry.Type, opts.Mode)
	}

	att := Attachment{
		AgentID:     agentID,
		MemoryID:    memoryID,
		Mode:        effectiveMode,
		Priority:    opts.Priority,
		Collections: append([]string(nil), opts.Collections...),
		AttachedAt:  s.now(),
	}
	s.attachments[agentID] = append(existing, att)
	s.rebuildInstanceScopesLocked(agentID)
	if err := s.persistStateLocked(); err != nil {
		return Attachment{}, err.Error(), err
	}
	return att, "memory attached", nil
}

// DetachMemory removes an attachment between an agent and memory package.
func (s *Store) DetachMemory(agentID, memoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attachments := s.attachments[agentID]
	idx := -1
	for i, a := range attachments {
		if a.MemoryID == memoryID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrAttachmentMissing
	}

	s.attachments[agentID] = append(attachments[:idx], attachments[idx+1:]...)
	for i := range s.mounts {
		if s.mounts[i].AgentID == agentID && s.mounts[i].MemoryID == memoryID {
			s.mounts = append(s.mounts[:i], s.mounts[i+1:]...)
			break
		}
	}
	s.rebuildInstanceScopesLocked(agentID)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

// ListAttachments returns all configured attachments for an agent.
func (s *Store) ListAttachments(agentID string) []Attachment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attachments := s.attachments[agentID]
	out := make([]Attachment, len(attachments))
	copy(out, attachments)
	return out
}

// SetAttachmentsFromLinks replaces an agent's attachments based on ordered memory IDs.
// The incoming order is preserved as ascending priority.
func (s *Store) SetAttachmentsFromLinks(agentID string, memoryIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attachments := make([]Attachment, 0, len(memoryIDs))
	hasPerAgent := false
	for i, memoryID := range memoryIDs {
		entry, ok := s.entries[memoryID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrMemoryNotFound, memoryID)
		}
		if entry.State == StateArchived {
			return fmt.Errorf("%w: %s is archived", ErrInvalidState, memoryID)
		}
		if entry.Type == TypePerAgent {
			if entry.Owner != "" && entry.Owner != agentID {
				return fmt.Errorf("%w: memory owner is %q, requester is %q", ErrOwnerMismatch, entry.Owner, agentID)
			}
			if hasPerAgent {
				return ErrPerAgentLimit
			}
			hasPerAgent = true
		}

		mode := s.policy.DefaultAccessMode(entry.Type)
		attachments = append(attachments, Attachment{
			AgentID:    agentID,
			MemoryID:   memoryID,
			Mode:       mode,
			Priority:   i,
			AttachedAt: s.now(),
		})
	}
	s.attachments[agentID] = attachments
	s.rebuildInstanceScopesLocked(agentID)
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	return nil
}

func normalizeAndSortScopes(scopes []Scope) []Scope {
	if len(scopes) == 0 {
		return nil
	}
	set := make(map[Scope]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = normalizeScope(scope)
		if scope == "" {
			continue
		}
		set[scope] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]Scope, 0, len(set))
	for scope := range set {
		out = append(out, scope)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) addManualScopeLocked(agentID string, scope Scope) bool {
	scope = normalizeScope(scope)
	if scope == "" {
		return false
	}
	existing := s.manualScopes[agentID]
	for _, v := range existing {
		if v == scope {
			return false
		}
	}
	existing = append(existing, scope)
	s.manualScopes[agentID] = normalizeAndSortScopes(existing)
	return true
}

func (s *Store) removeManualScopeLocked(agentID string, scope Scope) bool {
	scope = normalizeScope(scope)
	if scope == "" {
		return false
	}
	existing := s.manualScopes[agentID]
	if len(existing) == 0 {
		return false
	}
	updated := make([]Scope, 0, len(existing))
	removed := false
	for _, v := range existing {
		if v == scope {
			removed = true
			continue
		}
		updated = append(updated, v)
	}
	if !removed {
		return false
	}
	updated = normalizeAndSortScopes(updated)
	if len(updated) == 0 {
		delete(s.manualScopes, agentID)
		return true
	}
	s.manualScopes[agentID] = updated
	return true
}

func (s *Store) rebuildInstanceScopesLocked(agentID string) []Scope {
	set := make(map[Scope]struct{})
	for _, scope := range s.manualScopes[agentID] {
		scope = normalizeScope(scope)
		if scope == "" {
			continue
		}
		set[scope] = struct{}{}
	}
	for _, att := range s.attachments[agentID] {
		entry, ok := s.entries[att.MemoryID]
		if !ok {
			continue
		}
		scope := normalizeScope(scopeForEntry(entry))
		if scope == "" {
			continue
		}
		set[scope] = struct{}{}
	}
	scopes := make([]Scope, 0, len(set))
	for scope := range set {
		scopes = append(scopes, scope)
	}
	sort.SliceStable(scopes, func(i, j int) bool { return scopes[i] < scopes[j] })
	if len(scopes) == 0 {
		delete(s.instanceScopes, agentID)
		s.syncInstanceScopesToSQLiteLocked(agentID, nil)
		return nil
	}
	s.instanceScopes[agentID] = scopes
	s.syncInstanceScopesToSQLiteLocked(agentID, scopes)
	return scopes
}

// PrepareAgentMemory composes an effective memory view with deterministic precedence.
func (s *Store) PrepareAgentMemory(agentID string) (RuntimeMemoryContract, error) {
	// Serialize per-agent prepares so concurrent callers cannot race on
	// effective view filesystem state or mount rollback bookkeeping.
	releasePrepareLock := s.lockPrepareForAgent(agentID)
	defer releasePrepareLock()

	root, err := s.requireRootDir()
	if err != nil {
		return RuntimeMemoryContract{}, err
	}

	s.mu.RLock()
	attachments := append([]Attachment(nil), s.attachments[agentID]...)
	entries := make(map[string]Entry, len(attachments))
	manifests := make(map[string]PackageManifest, len(attachments))
	installPaths := make(map[string]string, len(attachments))
	runtimeReadTargetPath := s.runtimeReadTargetPath
	runtimeWriteTargetPath := s.runtimeWriteTargetPath
	for _, a := range attachments {
		entries[a.MemoryID] = s.entries[a.MemoryID]
		manifests[a.MemoryID] = s.manifests[a.MemoryID]
		installPaths[a.MemoryID] = s.installPath[a.MemoryID]
	}
	s.mu.RUnlock()
	if runtimeReadTargetPath == "" || runtimeWriteTargetPath == "" {
		defaultReadPath, defaultWritePath := defaultRuntimeMountTargets()
		if runtimeReadTargetPath == "" {
			runtimeReadTargetPath = defaultReadPath
		}
		if runtimeWriteTargetPath == "" {
			runtimeWriteTargetPath = defaultWritePath
		}
	}
	previousMounts := s.MountsForAgent(agentID)

	sorted := attachments
	sort.SliceStable(sorted, func(i, j int) bool {
		ei := entries[sorted[i].MemoryID]
		ej := entries[sorted[j].MemoryID]
		ri := regionOrder(ei.Type)
		rj := regionOrder(ej.Type)
		if ri != rj {
			return ri < rj
		}
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].MemoryID < sorted[j].MemoryID
	})

	inputDigest, err := computePrepareInputDigest(sorted, entries, manifests, installPaths)
	if err != nil {
		return RuntimeMemoryContract{}, err
	}

	s.mu.RLock()
	cachedDigest := s.viewInputDigest[agentID]
	cachedExplain, hasCachedExplain := s.views[agentID]
	s.mu.RUnlock()
	if hasCachedExplain && cachedDigest == inputDigest {
		if _, err := os.Stat(cachedExplain.ViewPath); err == nil {
			if _, err := os.Stat(cachedExplain.MountMapPath); err == nil {
				if err := s.applyMountStateWithRollback(agentID, sorted, previousMounts); err != nil {
					return RuntimeMemoryContract{}, err
				}
				contract := buildRuntimeContractFromExplain(agentID, runtimeReadTargetPath, runtimeWriteTargetPath, cachedExplain)
				s.recordAudit("", "", "prepare", agentID, auditResultSuccess, fmt.Sprintf("digest=%s reused=true", cachedExplain.Digest))
				return contract, nil
			}
		}
	}

	viewRoot := filepath.Join(root, "views", agentID)
	effectiveDir := filepath.Join(viewRoot, "effective")
	privateWriteDir := filepath.Join(viewRoot, "private")
	mountMapPath := filepath.Join(viewRoot, "mountmap.json")
	if err := os.RemoveAll(effectiveDir); err != nil {
		return RuntimeMemoryContract{}, fmt.Errorf("clear effective dir: %w", err)
	}
	if err := os.MkdirAll(effectiveDir, 0o755); err != nil {
		return RuntimeMemoryContract{}, fmt.Errorf("create effective dir: %w", err)
	}
	if err := os.MkdirAll(privateWriteDir, 0o755); err != nil {
		return RuntimeMemoryContract{}, fmt.Errorf("create private write dir: %w", err)
	}

	type fileOwner struct {
		MemoryID string
		Digest   string
	}
	pathOwners := map[string]fileOwner{}
	conflicts := make([]ViewConflict, 0)
	sources := make([]ViewSource, 0, len(sorted))

	for _, att := range sorted {
		entry := entries[att.MemoryID]
		manifest := manifests[att.MemoryID]
		installPath := installPaths[att.MemoryID]
		if installPath == "" {
			continue
		}

		selectedCollections, err := resolveCollectionPaths(manifest, att.Collections)
		if err != nil {
			return RuntimeMemoryContract{}, err
		}
		sourceContentRoot := filepath.Join(installPath, "content")
		files, err := listFiles(sourceContentRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return RuntimeMemoryContract{}, err
		}

		for _, abs := range files {
			rel, err := filepath.Rel(sourceContentRoot, abs)
			if err != nil {
				return RuntimeMemoryContract{}, fmt.Errorf("resolve content relative path: %w", err)
			}
			rel = filepath.ToSlash(rel)
			if !matchesCollectionSelection(rel, selectedCollections) {
				continue
			}
			fileDigest, err := computeFileDigest(abs)
			if err != nil {
				return RuntimeMemoryContract{}, err
			}
			target := filepath.Join(effectiveDir, filepath.FromSlash(rel))
			if prev, exists := pathOwners[rel]; exists && prev.MemoryID != att.MemoryID {
				if prev.Digest == fileDigest {
					// Same path and same digest: deterministic dedupe without conflict.
					continue
				}
				conflicts = append(conflicts, ViewConflict{
					Path:             rel,
					PreviousMemoryID: prev.MemoryID,
					PreviousDigest:   prev.Digest,
					CurrentMemoryID:  att.MemoryID,
					CurrentDigest:    fileDigest,
					WinnerMemoryID:   att.MemoryID,
					Resolution:       "current_wins_by_precedence",
				})
			}
			if err := copyFile(abs, target); err != nil {
				return RuntimeMemoryContract{}, err
			}
			pathOwners[rel] = fileOwner{MemoryID: att.MemoryID, Digest: fileDigest}
		}

		sources = append(sources, ViewSource{
			MemoryID:    att.MemoryID,
			Region:      entry.Type,
			Priority:    att.Priority,
			Collections: append([]string(nil), att.Collections...),
			SourcePath:  sourceContentRoot,
		})
	}

	digest, err := computeDirDigest(effectiveDir)
	if err != nil {
		return RuntimeMemoryContract{}, err
	}
	generatedAt := s.now()
	explain := ViewExplanation{
		AgentID:      agentID,
		ViewPath:     effectiveDir,
		MountMapPath: mountMapPath,
		Digest:       digest,
		GeneratedAt:  generatedAt,
		Sources:      sources,
		Conflicts:    conflicts,
	}
	if err := writeMountMap(explain); err != nil {
		return RuntimeMemoryContract{}, err
	}
	if err := s.applyMountStateWithRollback(agentID, sorted, previousMounts); err != nil {
		return RuntimeMemoryContract{}, err
	}

	s.mu.Lock()
	s.views[agentID] = explain
	s.viewInputDigest[agentID] = inputDigest
	if err := s.persistStateLocked(); err != nil {
		s.mu.Unlock()
		return RuntimeMemoryContract{}, err
	}
	s.mu.Unlock()

	contract := buildRuntimeContractFromExplain(agentID, runtimeReadTargetPath, runtimeWriteTargetPath, explain)
	s.recordAudit("", "", "prepare", agentID, auditResultSuccess, fmt.Sprintf("digest=%s reused=false", digest))
	return contract, nil
}

func (s *Store) applyMountStateWithRollback(agentID string, desired []Attachment, previous []MountRecord) error {
	s.UnmountAll(agentID)

	for _, att := range desired {
		if _, err := s.Mount(att.MemoryID, agentID, att.Mode); err != nil {
			s.UnmountAll(agentID)
			if rollbackErr := s.restoreMountRecords(previous); rollbackErr != nil {
				return fmt.Errorf("apply memory mounts failed: %v (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("apply memory mounts failed: %w", err)
		}
	}
	return nil
}

func (s *Store) restoreMountRecords(records []MountRecord) error {
	var errs []string
	for _, rec := range records {
		if _, err := s.Mount(rec.MemoryID, rec.AgentID, rec.AccessMode); err != nil && err != ErrAlreadyMounted {
			errs = append(errs, fmt.Sprintf("%s: %v", rec.MemoryID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// ExplainView returns the latest view composition details for the agent.
func (s *Store) ExplainView(agentID string) (ViewExplanation, error) {
	s.mu.RLock()
	explain, ok := s.views[agentID]
	s.mu.RUnlock()
	if !ok {
		return ViewExplanation{}, ErrViewNotPrepared
	}
	return explain, nil
}

func regionOrder(t Type) int {
	switch t {
	case TypePublic:
		return 0
	case TypeShared:
		return 1
	default:
		return 2
	}
}

func matchesCollectionSelection(rel string, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, c := range selected {
		if c == "" || rel == c || strings.HasPrefix(rel, c+"/") {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create target directory for %s: %w", dst, err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open target file %s: %w", dst, err)
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if closeErr != nil {
		return fmt.Errorf("close target file %s: %w", dst, closeErr)
	}
	return nil
}

func computeFileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read digest file %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func computeDirDigest(root string) (string, error) {
	files, err := listFiles(root)
	if err != nil {
		if os.IsNotExist(err) {
			h := sha256.Sum256(nil)
			return hex.EncodeToString(h[:]), nil
		}
		return "", err
	}
	h := sha256.New()
	for _, abs := range files {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return "", fmt.Errorf("resolve digest relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		h.Write([]byte(rel))
		h.Write([]byte{0})
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", fmt.Errorf("read digest file %s: %w", abs, err)
		}
		fileSum := sha256.Sum256(data)
		h.Write(fileSum[:])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeMountMap(explain ViewExplanation) error {
	if err := os.MkdirAll(filepath.Dir(explain.MountMapPath), 0o755); err != nil {
		return fmt.Errorf("create mountmap dir: %w", err)
	}
	payload := struct {
		AgentID   string         `json:"agent_id"`
		Generated string         `json:"generated_at"`
		Digest    string         `json:"digest"`
		ViewPath  string         `json:"view_path"`
		Sources   []ViewSource   `json:"sources"`
		Conflicts []ViewConflict `json:"conflicts"`
	}{
		AgentID:   explain.AgentID,
		Generated: explain.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Digest:    explain.Digest,
		ViewPath:  explain.ViewPath,
		Sources:   explain.Sources,
		Conflicts: explain.Conflicts,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mountmap: %w", err)
	}
	if err := os.WriteFile(explain.MountMapPath, b, 0o644); err != nil {
		return fmt.Errorf("write mountmap: %w", err)
	}
	return nil
}

func buildRuntimeContractFromExplain(agentID, runtimeReadTargetPath, runtimeWriteTargetPath string, explain ViewExplanation) RuntimeMemoryContract {
	env := map[string]string{
		"AGENTD_MEMORY_PATH":        runtimeReadTargetPath,
		"AGENTD_MEMORY_WRITE_PATH":  runtimeWriteTargetPath,
		"AGENTD_MEMORY_VIEW_DIGEST": explain.Digest,
	}
	privateWritePath := filepath.Join(filepath.Dir(explain.ViewPath), "private")
	return RuntimeMemoryContract{
		AgentID:          agentID,
		ViewPath:         explain.ViewPath,
		PrivateWritePath: privateWritePath,
		MountMapPath:     explain.MountMapPath,
		ViewDigest:       explain.Digest,
		Mounts: []RuntimeMount{
			{Source: explain.ViewPath, Target: env["AGENTD_MEMORY_PATH"], Mode: AccessReadOnly},
			{Source: privateWritePath, Target: env["AGENTD_MEMORY_WRITE_PATH"], Mode: AccessReadWrite},
		},
		Env:         env,
		Explanation: explain,
	}
}

func computePrepareInputDigest(sorted []Attachment, entries map[string]Entry, manifests map[string]PackageManifest, installPaths map[string]string) (string, error) {
	type prepareInput struct {
		MemoryID    string     `json:"memory_id"`
		Mode        AccessMode `json:"mode"`
		Priority    int        `json:"priority"`
		Collections []string   `json:"collections"`
		Digest      string     `json:"digest"`
		InstallPath string     `json:"install_path"`
	}

	inputs := make([]prepareInput, 0, len(sorted))
	for _, att := range sorted {
		manifest := manifests[att.MemoryID]
		inputs = append(inputs, prepareInput{
			MemoryID:    att.MemoryID,
			Mode:        att.Mode,
			Priority:    att.Priority,
			Collections: append([]string(nil), att.Collections...),
			Digest:      manifest.Provenance.Digest,
			InstallPath: installPaths[att.MemoryID],
		})
	}

	raw, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("marshal prepare input digest: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
