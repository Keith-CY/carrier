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
	ListEnabledSkillNames(ctx context.Context) ([]string, error)
	SetEnabledSkillNames(ctx context.Context, names []string) error
	ListSkillVersionPins(ctx context.Context) (map[string]string, error)
	SetSkillVersionPins(ctx context.Context, pins map[string]string) error
	ListSkillMetadata(ctx context.Context) (map[string]SkillLifecycleMetadata, error)
	SetSkillMetadata(ctx context.Context, metadata map[string]SkillLifecycleMetadata) error
}

type SkillLifecycleMetadata struct {
	Provenance  string `json:"provenance,omitempty"`
	InstalledAt string `json:"installedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type MemorySkillsStore struct {
	mu          sync.RWMutex
	installed   []string
	enabled     []string
	versionPins map[string]string
	metadata    map[string]SkillLifecycleMetadata
	configured  bool
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

func (s *MemorySkillsStore) ListEnabledSkillNames(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.configured {
		return append([]string(nil), s.installed...), nil
	}
	return append([]string(nil), s.enabled...), nil
}

func (s *MemorySkillsStore) SetEnabledSkillNames(_ context.Context, names []string) error {
	if s == nil {
		return fmt.Errorf("skills store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = normalizeSkillNames(names)
	s.configured = true
	return nil
}

func (s *MemorySkillsStore) ListSkillVersionPins(_ context.Context) (map[string]string, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSkillVersionPins(s.versionPins), nil
}

func (s *MemorySkillsStore) SetSkillVersionPins(_ context.Context, pins map[string]string) error {
	if s == nil {
		return fmt.Errorf("skills store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versionPins = normalizeSkillVersionPins(pins)
	return nil
}

func (s *MemorySkillsStore) ListSkillMetadata(_ context.Context) (map[string]SkillLifecycleMetadata, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSkillLifecycleMetadataMap(s.metadata), nil
}

func (s *MemorySkillsStore) SetSkillMetadata(_ context.Context, metadata map[string]SkillLifecycleMetadata) error {
	if s == nil {
		return fmt.Errorf("skills store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metadata = normalizeSkillLifecycleMetadataMap(metadata)
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
	existing, err := s.load()
	if err != nil {
		return err
	}
	state.Enabled = append([]string(nil), existing.Enabled...)
	state.EnabledConfigured = existing.EnabledConfigured
	state.VersionPins = cloneSkillVersionPins(existing.VersionPins)
	state.Metadata = cloneSkillLifecycleMetadataMap(existing.Metadata)
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

func (s *FileSkillsStore) ListEnabledSkillNames(_ context.Context) ([]string, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), state.Enabled...), nil
}

func (s *FileSkillsStore) SetEnabledSkillNames(_ context.Context, names []string) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("skills store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return err
	}
	state.Enabled = normalizeSkillNames(names)
	state.EnabledConfigured = true
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

func (s *FileSkillsStore) ListSkillVersionPins(_ context.Context) (map[string]string, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return nil, err
	}
	return cloneSkillVersionPins(state.VersionPins), nil
}

func (s *FileSkillsStore) SetSkillVersionPins(_ context.Context, pins map[string]string) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("skills store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return err
	}
	state.VersionPins = normalizeSkillVersionPins(pins)
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

func (s *FileSkillsStore) ListSkillMetadata(_ context.Context) (map[string]SkillLifecycleMetadata, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return nil, err
	}
	return cloneSkillLifecycleMetadataMap(state.Metadata), nil
}

func (s *FileSkillsStore) SetSkillMetadata(_ context.Context, metadata map[string]SkillLifecycleMetadata) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("skills store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.load()
	if err != nil {
		return err
	}
	state.Metadata = normalizeSkillLifecycleMetadataMap(metadata)
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
	Installed         []string                          `json:"installed"`
	Enabled           []string                          `json:"enabled"`
	EnabledConfigured bool                              `json:"enabledConfigured,omitempty"`
	VersionPins       map[string]string                 `json:"versionPins,omitempty"`
	Metadata          map[string]SkillLifecycleMetadata `json:"metadata,omitempty"`
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
	state.Enabled = normalizeSkillNames(state.Enabled)
	state.VersionPins = normalizeSkillVersionPins(state.VersionPins)
	state.Metadata = normalizeSkillLifecycleMetadataMap(state.Metadata)
	if !state.EnabledConfigured {
		state.Enabled = append([]string(nil), state.Installed...)
	}
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

func normalizeSkillVersionPins(pins map[string]string) map[string]string {
	if len(pins) == 0 {
		return nil
	}
	out := make(map[string]string, len(pins))
	for name, version := range pins {
		normalizedName := strings.TrimSpace(strings.ToLower(name))
		normalizedVersion := strings.TrimSpace(version)
		if normalizedName == "" || normalizedVersion == "" {
			continue
		}
		out[normalizedName] = normalizedVersion
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneSkillVersionPins(pins map[string]string) map[string]string {
	if len(pins) == 0 {
		return nil
	}
	out := make(map[string]string, len(pins))
	for name, version := range pins {
		out[name] = version
	}
	return out
}

func cloneSkillLifecycleMetadataMap(metadata map[string]SkillLifecycleMetadata) map[string]SkillLifecycleMetadata {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]SkillLifecycleMetadata, len(metadata))
	for name, meta := range metadata {
		normalizedName := strings.TrimSpace(strings.ToLower(name))
		if normalizedName == "" {
			continue
		}
		out[normalizedName] = normalizeSkillLifecycleMetadata(meta)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSkillLifecycleMetadataMap(metadata map[string]SkillLifecycleMetadata) map[string]SkillLifecycleMetadata {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]SkillLifecycleMetadata, len(metadata))
	for name, meta := range metadata {
		normalizedName := strings.TrimSpace(strings.ToLower(name))
		if normalizedName == "" {
			continue
		}
		out[normalizedName] = normalizeSkillLifecycleMetadata(meta)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSkillLifecycleMetadata(metadata SkillLifecycleMetadata) SkillLifecycleMetadata {
	metadata.Provenance = strings.TrimSpace(metadata.Provenance)
	metadata.InstalledAt = strings.TrimSpace(metadata.InstalledAt)
	metadata.UpdatedAt = strings.TrimSpace(metadata.UpdatedAt)
	return metadata
}
