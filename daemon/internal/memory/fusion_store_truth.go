package memory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
