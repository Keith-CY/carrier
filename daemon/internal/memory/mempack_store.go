package memory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
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

	s.mu.Lock()
	if _, exists := s.entries[memoryID]; exists {
		s.mu.Unlock()
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, "memory already exists")
		return Entry{}, fmt.Errorf("memory %q already exists", memoryID)
	}
	s.mu.Unlock()

	if err := os.RemoveAll(installPath); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("clear install path: %w", err)
	}
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, fmt.Errorf("create install path: %w", err)
	}
	if err := extractZipInto(&zr.Reader, installPath); err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "import", memoryID, auditResultFailure, err.Error())
		return Entry{}, err
	}

	if err := verifyMempackDigest(mempackPath, manifest.Provenance.Digest); err != nil {
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
	s.entries[memoryID] = entry
	s.manifests[memoryID] = manifest
	s.installPath[memoryID] = installPath
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

	s.mu.RLock()
	manifest, ok := s.manifests[memoryID]
	installPath := s.installPath[memoryID]
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

	zw := zip.NewWriter(f)
	defer zw.Close()

	files, err := listFiles(installPath)
	if err != nil {
		s.recordAudit(opts.RequestID, opts.Actor, "export", memoryID, auditResultFailure, err.Error())
		return "", err
	}

	for _, abs := range files {
		rel, err := filepath.Rel(installPath, abs)
		if err != nil {
			return "", fmt.Errorf("calculate export relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if !shouldExportPath(rel, selectedCollections) {
			continue
		}

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
		name := filepath.Clean(f.Name)
		if name == "." {
			continue
		}
		target := filepath.Join(dstRoot, name)
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
