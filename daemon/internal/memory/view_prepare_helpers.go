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
