package work

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var userHomeDirFunc = os.UserHomeDir

type Roots struct {
	Root     string
	App      string
	Projects string
	Works    string
}

func ResolveRoots() (Roots, error) {
	root, err := resolveRootPath()
	if err != nil {
		return Roots{}, err
	}
	return Roots{
		Root:     root,
		App:      firstNonEmptyPath(os.Getenv("CARRIER_APP_ROOT"), filepath.Join(root, "app")),
		Projects: firstNonEmptyPath(os.Getenv("CARRIER_PROJECTS_ROOT"), filepath.Join(root, "projects")),
		Works:    firstNonEmptyPath(os.Getenv("CARRIER_WORKS_ROOT"), filepath.Join(root, "works")),
	}, nil
}

func resolveRootPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_ROOT")); path != "" {
		return filepath.Clean(path), nil
	}
	home, err := resolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier"), nil
}

func resolveHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Clean(home), nil
	}
	home, err := userHomeDirFunc()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	trimmed := strings.TrimSpace(home)
	if trimmed == "" {
		return "", fmt.Errorf("resolve home dir: empty home dir")
	}
	return filepath.Clean(trimmed), nil
}

func firstNonEmptyPath(raw string, fallback string) string {
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(fallback)
}
