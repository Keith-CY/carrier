package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
