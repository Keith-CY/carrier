package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type managedIsolationFile struct {
	hostPath  string
	guestDir  string
	guestPath string
}

func managedIsolationFilesForAgent(agentID string) ([]managedIsolationFile, error) {
	home, err := isolationUserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "zeroclaw":
		return []managedIsolationFile{
			{
				hostPath:  filepath.Join(home, ".zeroclaw", "config.toml"),
				guestDir:  "$HOME/.zeroclaw",
				guestPath: "$HOME/.zeroclaw/config.toml",
			},
		}, nil
	default:
		return nil, nil
	}
}

func (s *Service) syncManagedIsolationFiles(ctx context.Context, agentID string, backend isolationBackend) error {
	if backend == nil {
		return nil
	}
	files, err := managedIsolationFilesForAgent(agentID)
	if err != nil {
		return err
	}
	for _, file := range files {
		raw, readErr := os.ReadFile(file.hostPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return fmt.Errorf("read managed config %q: %w", file.hostPath, readErr)
		}
		command, buildErr := buildManagedIsolationFileSyncCommand(file.guestDir, file.guestPath, raw)
		if buildErr != nil {
			return buildErr
		}
		wrapped, wrapErr := backend.WrapCommand(command)
		if wrapErr != nil {
			return wrapErr
		}
		result, _, runErr := s.runCommandWithAgentLogs(ctx, agentID, "isolation-sync-config", wrapped)
		if runErr != nil {
			return fmt.Errorf("sync managed config into isolated runtime: %w", runErr)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("sync managed config into isolated runtime exited with %d", result.ExitCode)
		}
		s.appendLog(agentID, fmt.Sprintf("[isolation-sync-config] synced %s into guest", filepath.Base(file.hostPath)))
	}
	return nil
}

func buildManagedIsolationFileSyncCommand(guestDir, guestPath string, raw []byte) (string, error) {
	dir := strings.TrimSpace(guestDir)
	target := strings.TrimSpace(guestPath)
	if dir == "" || target == "" {
		return "", fmt.Errorf("%w: managed isolation guest path is empty", ErrIsolationUnavailable)
	}
	safeDir, err := shellManagedIsolationGuestPath(dir)
	if err != nil {
		return "", err
	}
	safeTarget, err := shellManagedIsolationGuestPath(target)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"set -e",
		"umask 077",
		fmt.Sprintf("mkdir -p %s", safeDir),
		fmt.Sprintf("printf %%s %s > %s", shellSingleQuote(string(raw)), safeTarget),
		fmt.Sprintf("chmod 600 %s", safeTarget),
	}, "; "), nil
}

func shellManagedIsolationGuestPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("%w: managed isolation guest path is empty", ErrIsolationUnavailable)
	}
	if trimmed == "$HOME" {
		return `"$HOME"`, nil
	}
	if strings.HasPrefix(trimmed, "$HOME/") {
		suffix := strings.TrimPrefix(trimmed, "$HOME")
		if strings.ContainsAny(suffix, "\"`") {
			return "", fmt.Errorf("%w: unsupported managed isolation guest path %q", ErrIsolationUnavailable, trimmed)
		}
		return `"$HOME` + suffix + `"`, nil
	}
	return shellSingleQuote(trimmed), nil
}
