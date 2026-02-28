package lifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const rollbackSnapshotTTL = 24 * time.Hour

func snapshotAgentState(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agentID is required")
	}
	snapshotDir, err := rollbackSnapshotDir(agentID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(snapshotDir); err != nil {
		return err
	}
	if err := os.MkdirAll(snapshotDir, 0o700); err != nil {
		return err
	}

	stateSrc := filepath.Join(agentStateDir(), agentID+".json")
	stateDst := filepath.Join(snapshotDir, "state.json")
	if err := copyIfExists(stateSrc, stateDst); err != nil {
		return err
	}

	binSrc := filepath.Join(agentBinaryDir(), agentID)
	binDst := filepath.Join(snapshotDir, "binary")
	if err := copyIfExists(binSrc, binDst); err != nil {
		return err
	}

	marker := filepath.Join(snapshotDir, "snapshot_at")
	if err := os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		return err
	}
	return nil
}

func restoreAgentState(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agentID is required")
	}
	snapshotDir, err := rollbackSnapshotDir(agentID)
	if err != nil {
		return err
	}

	stateDst := filepath.Join(agentStateDir(), agentID+".json")
	stateSrc := filepath.Join(snapshotDir, "state.json")
	if err := copyIfExists(stateSrc, stateDst); err != nil {
		return err
	}

	binDst := filepath.Join(agentBinaryDir(), agentID)
	binSrc := filepath.Join(snapshotDir, "binary")
	if err := copyIfExists(binSrc, binDst); err != nil {
		return err
	}
	return nil
}

func cleanupRollbackSnapshot(agentID string) error {
	snapshotDir, err := rollbackSnapshotDir(agentID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(snapshotDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cleanupExpiredRollbackSnapshots() error {
	root, err := rollbackRootDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-rollbackSnapshotTTL)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(path)
		}
	}
	return nil
}

func rollbackRootDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_ROLLBACK_DIR")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "rollback"), nil
}

func rollbackSnapshotDir(agentID string) (string, error) {
	root, err := rollbackRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, strings.TrimSpace(agentID)), nil
}

func agentStateDir() string {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_AGENT_STATE_DIR")); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".carrier", "agents")
}

func agentBinaryDir() string {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_AGENT_BINARY_DIR")); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".carrier", "bin")
}

func copyIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}
