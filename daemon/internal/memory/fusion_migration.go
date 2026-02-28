package memory

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrationValidation summarizes consistency between in-memory state and SQLite cache.
type MigrationValidation struct {
	Records        int    `json:"records"`
	Observations   int    `json:"observations"`
	Grants         int    `json:"grants"`
	InstanceScopes int    `json:"instance_scopes"`
	SQLiteRecords  int    `json:"sqlite_records"`
	Consistent     bool   `json:"consistent"`
	Message        string `json:"message"`
}

// CreateMigrationBackup creates a point-in-time backup archive for rollback.
func (s *Store) CreateMigrationBackup(actor, requestID string) (string, error) {
	root, err := s.requireRootDir()
	if err != nil {
		return "", err
	}
	s.mu.RLock()
	locked := true
	defer func() {
		if locked {
			s.mu.RUnlock()
		}
	}()

	backupDir := filepath.Join(root, "artifacts", "migration")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("fusionmem-backup-%s.zip", s.now().UTC().Format("20060102T150405Z"))
	path := filepath.Join(backupDir, name)
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	paths := []string{
		filepath.Join(root, "state"),
		filepath.Join(root, "truth"),
		filepath.Join(root, "index"),
		filepath.Join(root, "packages"),
	}
	for _, abs := range paths {
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if err := addPathToZip(zw, abs, filepath.Base(abs)); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	locked = false
	s.mu.RUnlock()
	s.recordAudit(requestID, actor, "migration_backup", path, auditResultSuccess, "backup created")
	return path, nil
}

// ValidateMigration verifies local state and index consistency.
func (s *Store) ValidateMigration() MigrationValidation {
	s.mu.RLock()
	v := MigrationValidation{
		Records:      len(s.records),
		Observations: len(s.observations),
		Grants:       len(s.grants),
	}
	for _, scopes := range s.instanceScopes {
		v.InstanceScopes += len(scopes)
	}
	s.mu.RUnlock()

	if !s.ensureSQLiteIndexLockedForRead() {
		v.Consistent = true
		v.Message = "sqlite cache unavailable; treated as rebuildable cache"
		return v
	}

	s.mu.RLock()
	db, err := s.openSQLiteLocked()
	s.mu.RUnlock()
	if err != nil {
		v.Consistent = false
		v.Message = err.Error()
		return v
	}
	defer db.Close()
	_ = db.QueryRow(`SELECT COUNT(1) FROM memory_records WHERE archived_at IS NULL`).Scan(&v.SQLiteRecords)
	v.Consistent = v.SQLiteRecords >= v.Records
	if v.Consistent {
		v.Message = "validation passed"
	} else {
		v.Message = "sqlite index count is lower than in-memory records"
	}
	return v
}

// RollbackFromBackup restores state/truth/index/packages from backup archive.
func (s *Store) RollbackFromBackup(backupPath, actor, requestID string) error {
	root, err := s.requireRootDir()
	if err != nil {
		return err
	}
	backupPath = strings.TrimSpace(backupPath)
	if backupPath == "" {
		return fmt.Errorf("backup path is required")
	}

	zr, err := zip.OpenReader(backupPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	tmpRoot := filepath.Join(root, ".tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(tmpRoot, "fusionmem-rollback-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractZipInto(&zr.Reader, staging); err != nil {
		return err
	}

	s.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.mu.Unlock()
		}
	}()
	restoreFolders := []string{"state", "truth", "index", "packages"}
	for _, folder := range restoreFolders {
		src := filepath.Join(staging, folder)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(root, folder)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}

	s.entries = make(map[string]Entry)
	s.mounts = nil
	s.manifests = make(map[string]PackageManifest)
	s.installPath = make(map[string]string)
	s.attachments = make(map[string][]Attachment)
	s.views = make(map[string]ViewExplanation)
	s.viewInputDigest = make(map[string]string)
	s.records = make(map[string]MemoryRecord)
	s.observations = nil
	s.grants = make(map[string]Grant)
	s.instanceScopes = make(map[string][]Scope)
	s.sqliteReady = false
	s.sqliteFTSEnabled = false
	if err := s.loadState(); err != nil {
		return err
	}
	s.migrateLegacyToFusionLocked()
	s.gcObservationsLocked()
	s.rebuildSQLiteIndexLocked()
	if err := s.persistStateLocked(); err != nil {
		return err
	}
	locked = false
	s.mu.Unlock()
	s.recordAudit(requestID, actor, "migration_rollback", backupPath, auditResultSuccess, "rollback completed")
	return nil
}

func (s *Store) ensureSQLiteIndexLockedForRead() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureSQLiteIndexLocked()
}
