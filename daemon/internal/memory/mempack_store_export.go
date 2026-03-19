package memory

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
		return nil
	}
	needed := uint64(requiredBytes)
	if available < needed {
		return fmt.Errorf("%w: required=%d available=%d", ErrDiskSpaceLow, needed, available)
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

func sanitizeID(id string) string {
	replacer := strings.NewReplacer("/", "_", "@", "_", ":", "_", "\\", "_")
	return replacer.Replace(id)
}
