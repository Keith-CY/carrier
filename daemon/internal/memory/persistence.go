package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type persistedStoreState struct {
	Entries         map[string]Entry           `json:"entries"`
	Mounts          []MountRecord              `json:"mounts"`
	Manifests       map[string]PackageManifest `json:"manifests"`
	InstallPath     map[string]string          `json:"install_path"`
	Attachments     map[string][]Attachment    `json:"attachments"`
	Views           map[string]ViewExplanation `json:"views"`
	ViewInputDigest map[string]string          `json:"view_input_digest"`
}

func (s *Store) loadState() error {
	statePath := strings.TrimSpace(s.statePath)
	if statePath == "" {
		return nil
	}

	raw, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read memory state: %w", err)
	}

	var state persistedStoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("parse memory state: %w", err)
	}

	if state.Entries != nil {
		s.entries = state.Entries
	}
	if state.Mounts != nil {
		s.mounts = state.Mounts
	}
	if state.Manifests != nil {
		s.manifests = state.Manifests
	}
	if state.InstallPath != nil {
		s.installPath = state.InstallPath
	}
	if state.Attachments != nil {
		s.attachments = state.Attachments
	}
	if state.Views != nil {
		s.views = state.Views
	}
	if state.ViewInputDigest != nil {
		s.viewInputDigest = state.ViewInputDigest
	}

	return nil
}

func (s *Store) persistStateLocked() error {
	statePath := strings.TrimSpace(s.statePath)
	if statePath == "" {
		return nil
	}

	state := persistedStoreState{
		Entries:         make(map[string]Entry, len(s.entries)),
		Mounts:          append([]MountRecord(nil), s.mounts...),
		Manifests:       make(map[string]PackageManifest, len(s.manifests)),
		InstallPath:     make(map[string]string, len(s.installPath)),
		Attachments:     make(map[string][]Attachment, len(s.attachments)),
		Views:           make(map[string]ViewExplanation, len(s.views)),
		ViewInputDigest: make(map[string]string, len(s.viewInputDigest)),
	}
	for k, v := range s.entries {
		state.Entries[k] = v
	}
	for k, v := range s.manifests {
		state.Manifests[k] = v
	}
	for k, v := range s.installPath {
		state.InstallPath[k] = v
	}
	for k, v := range s.attachments {
		state.Attachments[k] = append([]Attachment(nil), v...)
	}
	for k, v := range s.views {
		state.Views[k] = v
	}
	for k, v := range s.viewInputDigest {
		state.ViewInputDigest[k] = v
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		s.lastStateErr = fmt.Errorf("create memory state dir: %w", err)
		return s.lastStateErr
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		s.lastStateErr = fmt.Errorf("marshal memory state: %w", err)
		return s.lastStateErr
	}

	tmp, err := os.CreateTemp(filepath.Dir(statePath), "memory-state-*.tmp")
	if err != nil {
		s.lastStateErr = fmt.Errorf("create memory state temp file: %w", err)
		return s.lastStateErr
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		s.lastStateErr = fmt.Errorf("write memory state temp file: %w", err)
		return s.lastStateErr
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		s.lastStateErr = fmt.Errorf("fsync memory state temp file: %w", err)
		return s.lastStateErr
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		s.lastStateErr = fmt.Errorf("close memory state temp file: %w", err)
		return s.lastStateErr
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		s.lastStateErr = fmt.Errorf("replace memory state: %w", err)
		return s.lastStateErr
	}

	s.lastStateErr = nil
	return nil
}
