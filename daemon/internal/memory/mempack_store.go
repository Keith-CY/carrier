package memory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`)

// ImportMemory installs a mempack artifact into the configured region store.
func (s *Store) ImportMemory(mempackPath string, opts ImportOptions) (Entry, error) {
	root, err := s.requireRootDir()
	if err != nil {
		return Entry{}, err
	}

	zr, err := zip.OpenReader(mempackPath)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", mempackPath, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("open mempack: %w", err)
	}
	defer zr.Close()

	manifestBytes, err := zipEntryBytes(&zr.Reader, "memory.yaml")
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", mempackPath, auditResultFailure, err.Error())
		return Entry{}, err
	}
	manifest, err := parseManifest(manifestBytes)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", mempackPath, auditResultFailure, err.Error())
		return Entry{}, err
	}

	region := manifest.Region
	if opts.TargetRegion != "" {
		region = opts.TargetRegion
	}
	region = normalizeRegion(string(region))

	publisher := strings.TrimSpace(opts.Publisher)
	if publisher == "" {
		publisher = strings.TrimSpace(manifest.Publisher)
	}
	owner := strings.TrimSpace(opts.Owner)
	if region == TypePublic && publisher == "" {
		publisher = "unknown"
	}
	if region == TypePerAgent && owner == "" {
		s.recordAudit(opts.RequestID, opts.Actor, "import", mempackPath, auditResultFailure, "owner required for private memory")
		return Entry{}, fmt.Errorf("owner is required when importing private memory")
	}

	memoryID := memoryIDFor(region, owner, publisher, manifest.ID, manifest.Version)
	installPath := installPathFor(root, region, owner, publisher, manifest.ID, manifest.Version)
	if err := verifyMempackDigest(mempackPath, manifest.Provenance.Digest); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, err
	}

	tmpRoot := filepath.Join(root, "packages", ".tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("create temporary import root: %w", err)
	}
	stagingPath, err := os.MkdirTemp(tmpRoot, "mempack-import-*")
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("create temporary import dir: %w", err)
	}
	staged := true
	defer func() {
		if staged {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	if err := extractZipInto(&zr.Reader, stagingPath); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, err
	}

	now := s.now()
	entry := Entry{
		ID:        memoryID,
		Name:      manifest.Name,
		Version:   manifest.Version,
		Type:      region,
		Owner:     owner,
		State:     StateCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	if _, exists := s.entries[memoryID]; exists {
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, "memory already exists")
		return Entry{}, fmt.Errorf("memory %q already exists", memoryID)
	}
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("create install parent path: %w", err)
	}
	if err := os.RemoveAll(installPath); err != nil {
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("clear install path: %w", err)
	}
	if err := os.Rename(stagingPath, installPath); err != nil {
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("activate imported memory package: %w", err)
	}
	staged = false
	s.entries[memoryID] = entry
	s.manifests[memoryID] = manifest
	s.installPath[memoryID] = installPath
	if scope := scopeForEntry(entry); scope != "" {
		s.records[memoryID] = MemoryRecord{
			ID:             memoryID,
			Scope:          scope,
			Type:           RecordTypeNote,
			ContentSummary: strings.TrimSpace(manifest.Name),
			Provenance:     "import:" + mempackPath,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		s.syncRecordToSQLiteLocked(s.records[memoryID])
		_ = s.writeStableTruthRecordLocked(s.records[memoryID])
	}
	if err := s.persistStateLocked(); err != nil {
		delete(s.entries, memoryID)
		delete(s.manifests, memoryID)
		delete(s.installPath, memoryID)
		delete(s.records, memoryID)
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, err
	}
	s.mu.Unlock()

	s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultSuccess, "memory imported")
	return entry, nil
}

// ExportMemory creates a .mempack.zip artifact from an installed memory package.
func (s *Store) ExportMemory(memoryID string, opts ExportOptions) (string, error) {
	root, err := s.requireRootDir()
	if err != nil {
		return "", err
	}
	releaseExport, err := s.acquireExportSlot()
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", err
	}
	defer releaseExport()

	s.mu.RLock()
	manifest, ok := s.manifests[memoryID]
	installPath := s.installPath[memoryID]
	exportMaxBytes := s.exportMaxBytes
	s.mu.RUnlock()
	if !ok {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, ErrMemoryNotFound.Error())
		return "", ErrMemoryNotFound
	}

	selectedCollections, err := resolveCollectionPaths(manifest, opts.Collections)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", err
	}

	exportsDir := filepath.Join(root, "artifacts", "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", fmt.Errorf("create exports dir: %w", err)
	}
	fileName := fmt.Sprintf("%s.mempack.zip", sanitizeID(memoryID))
	exportPath := filepath.Join(exportsDir, fileName)

	f, err := os.Create(exportPath)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", fmt.Errorf("create export artifact: %w", err)
	}
	defer f.Close()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(exportPath)
		}
	}()

	zw := zip.NewWriter(f)

	files, err := listFiles(installPath)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", err
	}

	type exportCandidate struct {
		abs string
		rel string
	}
	candidates := make([]exportCandidate, 0, len(files))
	var totalBytes int64

	for _, abs := range files {
		rel, err := filepath.Rel(installPath, abs)
		if err != nil {
			return "", fmt.Errorf("calculate export relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if !shouldExportPath(rel, selectedCollections) {
			continue
		}
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return "", fmt.Errorf("stat export file %s: %w", rel, statErr)
		}
		totalBytes += info.Size()
		if exportMaxBytes > 0 && totalBytes > exportMaxBytes {
			err := fmt.Errorf("%w: total=%d limit=%d", ErrExportTooLarge, totalBytes, exportMaxBytes)
			s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
			return "", err
		}
		candidates = append(candidates, exportCandidate{abs: abs, rel: rel})
	}
	if err := ensureDiskSpaceForExport(exportsDir, totalBytes); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", err
	}

	for _, file := range candidates {
		rel := file.rel
		abs := file.abs
		raw, err := os.ReadFile(abs)
		if err != nil {
			return "", fmt.Errorf("read export file %s: %w", rel, err)
		}
		w, err := zw.Create(rel)
		if err != nil {
			return "", fmt.Errorf("create export entry %s: %w", rel, err)
		}
		if _, err := w.Write(raw); err != nil {
			return "", fmt.Errorf("write export entry %s: %w", rel, err)
		}
	}

	if err := zw.Close(); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", fmt.Errorf("close export artifact: %w", err)
	}
	cleanup = false

	s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultSuccess, "memory exported")
	return exportPath, nil
}

// AuditLogs returns a stable snapshot of memory operation audit events.
func (s *Store) AuditLogs() []AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditEvent, len(s.audits))
	copy(out, s.audits)
	return out
}

func (s *Store) requireRootDir() (string, error) {
	s.mu.RLock()
	root := strings.TrimSpace(s.rootDir)
	s.mu.RUnlock()
	if root == "" {
		return "", ErrRootDirRequired
	}
	return root, nil
}

func (s *Store) acquireExportSlot() (func(), error) {
	s.mu.RLock()
	slots := s.exportSlots
	s.mu.RUnlock()
	if slots == nil {
		return func() {}, nil
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	default:
		return nil, ErrExportBusy
	}
}

func ensureDiskSpaceForExport(path string, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return nil
	}
	available, err := availableDiskBytes(path)
	if err != nil {
		// Best effort: when disk availability cannot be determined on this platform,
		// continue and rely on filesystem write errors.
		return nil
	}
	needed := uint64(requiredBytes)
	if available < needed {
		return fmt.Errorf("%w: required=%d available=%d", ErrDiskSpaceLow, needed, available)
	}
	return nil
}

func (s *Store) recordAudit(requestID, actor, action, target, result, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := AuditEvent{
		RequestID: requestID,
		Actor:     actor,
		Action:    action,
		Target:    target,
		Result:    result,
		Message:   message,
		Timestamp: s.now(),
	}
	s.audits = append(s.audits, e)
	if len(s.audits) > s.auditLimit {
		over := len(s.audits) - s.auditLimit
		s.audits = append([]AuditEvent(nil), s.audits[over:]...)
	}
}

func verifyMempackDigest(path, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" || !sha256DigestPattern.MatchString(expected) {
		return nil
	}
	if strings.EqualFold(expected, "sha256:"+strings.Repeat("0", 64)) {
		// Placeholder digest allows import while preserving schema validity.
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open mempack for digest: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash mempack: %w", err)
	}
	got := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(expected, got) {
		return fmt.Errorf("mempack digest mismatch: expected %s got %s", expected, got)
	}
	return nil
}

func zipEntryBytes(r *zip.Reader, name string) ([]byte, error) {
	for _, f := range r.File {
		if filepath.ToSlash(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %s: %w", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("read zip entry %s: %w", name, err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("missing required zip entry %q", name)
}

func extractZipInto(r *zip.Reader, dstRoot string) error {
	for _, f := range r.File {
		cleanArchivePath, err := cleanArchiveEntryPath(f.Name)
		if err != nil {
			return err
		}
		if cleanArchivePath == "." {
			continue
		}
		target := filepath.Join(dstRoot, filepath.FromSlash(cleanArchivePath))
		if !isWithinRoot(dstRoot, target) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip file %s: %w", f.Name, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileModeOrDefault(f.Mode()))
		if err != nil {
			rc.Close()
			return fmt.Errorf("create file %s: %w", target, err)
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("extract file %s: %w", target, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close extracted file %s: %w", target, closeErr)
		}
	}
	return nil
}

func cleanArchiveEntryPath(raw string) (string, error) {
	normalized := strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("zip entry %q must be relative", raw)
	}
	if strings.Contains(normalized, "\x00") {
		return "", fmt.Errorf("zip entry %q contains null byte", raw)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", fmt.Errorf("zip entry %q contains parent traversal", raw)
		}
	}
	clean := path.Clean(normalized)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("zip entry %q escapes destination", raw)
	}
	if len(clean) >= 2 && clean[1] == ':' &&
		((clean[0] >= 'a' && clean[0] <= 'z') || (clean[0] >= 'A' && clean[0] <= 'Z')) {
		return "", fmt.Errorf("zip entry %q contains a drive-letter path", raw)
	}
	return clean, nil
}

func listFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk files under %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

func shouldExportPath(rel string, selectedCollections []string) bool {
	if rel == "memory.yaml" || rel == "README.md" || rel == "LICENSE" || strings.HasPrefix(rel, "meta/") {
		return true
	}
	if !strings.HasPrefix(rel, "content/") {
		return len(selectedCollections) == 0
	}
	if len(selectedCollections) == 0 {
		return true
	}
	contentRel := strings.TrimPrefix(rel, "content/")
	for _, c := range selectedCollections {
		if contentRel == c || strings.HasPrefix(contentRel, c+"/") {
			return true
		}
	}
	return false
}

func resolveCollectionPaths(m PackageManifest, selected []string) ([]string, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	lookup := make(map[string]string, len(m.Collections))
	for _, c := range m.Collections {
		lookup[c.ID] = normalizeCollectionPath(c.Path)
	}
	out := make([]string, 0, len(selected))
	for _, id := range selected {
		path, ok := lookup[id]
		if !ok {
			return nil, fmt.Errorf("collection %q not found", id)
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeCollectionPath(path string) string {
	p := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "content/")
	if p == "." {
		return ""
	}
	return p
}

func memoryIDFor(region Type, owner, publisher, id, version string) string {
	switch normalizeRegion(string(region)) {
	case TypePublic:
		return fmt.Sprintf("public/%s/%s@%s", publisher, id, version)
	case TypeShared:
		return fmt.Sprintf("shared/%s@%s", id, version)
	default:
		return fmt.Sprintf("private/%s/%s@%s", owner, id, version)
	}
}

func installPathFor(root string, region Type, owner, publisher, id, version string) string {
	fileRef := fmt.Sprintf("%s@%s", id, version)
	switch normalizeRegion(string(region)) {
	case TypePublic:
		return filepath.Join(root, "packages", "public", publisher, fileRef)
	case TypeShared:
		return filepath.Join(root, "packages", "shared", fileRef)
	default:
		return filepath.Join(root, "packages", "private", owner, fileRef)
	}
}

func sanitizeID(id string) string {
	replacer := strings.NewReplacer("/", "_", "@", "_", ":", "_", "\\", "_")
	return replacer.Replace(id)
}

func isWithinRoot(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanRoot == cleanPath {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator))
}

func fileModeOrDefault(mode fs.FileMode) fs.FileMode {
	if mode == 0 {
		return 0o644
	}
	return mode
}
