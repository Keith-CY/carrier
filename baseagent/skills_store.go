package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SkillsStore interface {
	ListInstalledSkillNames(ctx context.Context) ([]string, error)
	SetInstalledSkillNames(ctx context.Context, names []string) error
}

type MemorySkillsStore struct {
	mu        sync.RWMutex
	installed []string
}

func NewMemorySkillsStore() *MemorySkillsStore {
	return &MemorySkillsStore{}
}

func (s *MemorySkillsStore) ListInstalledSkillNames(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.installed...), nil
}

func (s *MemorySkillsStore) SetInstalledSkillNames(_ context.Context, names []string) error {
	if s == nil {
		return fmt.Errorf("skills store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installed = normalizeSkillNames(names)
	return nil
}

type FileSkillsStore struct {
	path string
	mu   sync.Mutex
}

func NewFileSkillsStore(path string) *FileSkillsStore {
	return &FileSkillsStore{path: strings.TrimSpace(path)}
}

func (s *FileSkillsStore) ListInstalledSkillNames(_ context.Context) ([]string, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), state.Installed...), nil
}

func (s *FileSkillsStore) SetInstalledSkillNames(_ context.Context, names []string) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("skills store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state := fileSkillsStoreState{
		Installed: normalizeSkillNames(names),
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create skills store directory: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skills store: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("write skills store: %w", err)
	}
	return nil
}

type fileSkillsStoreState struct {
	Installed []string `json:"installed"`
}

func (s *FileSkillsStore) load() (fileSkillsStoreState, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSkillsStoreState{}, nil
		}
		return fileSkillsStoreState{}, fmt.Errorf("read skills store: %w", err)
	}
	var state fileSkillsStoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fileSkillsStoreState{}, fmt.Errorf("parse skills store: %w", err)
	}
	state.Installed = normalizeSkillNames(state.Installed)
	return state, nil
}

func normalizeSkillNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
