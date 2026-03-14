package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type workProjectsState struct {
	Projects []work.Project `json:"projects"`
}

var (
	workProjectsStoreMu        sync.Mutex
	writeWorkProjectsStoreFile = os.WriteFile
)

func workProjectsStorePath() (string, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return "", err
	}
	return filepath.Join(roots.Works, "projects.json"), nil
}

func loadWorkProjectsState() (*workProjectsState, string, error) {
	path, err := workProjectsStorePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &workProjectsState{Projects: []work.Project{}}, path, nil
		}
		return nil, "", fmt.Errorf("read work projects store: %w", err)
	}
	var state workProjectsState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, "", fmt.Errorf("parse work projects store: %w", err)
	}
	if state.Projects == nil {
		state.Projects = []work.Project{}
	}
	return &state, path, nil
}

func saveWorkProjectsState(path string, state *workProjectsState) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("work projects store path is empty")
	}
	if state == nil {
		return errors.New("work projects state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create work projects dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal work projects store: %w", err)
	}
	if err := writeWorkProjectsStoreFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write work projects store: %w", err)
	}
	return nil
}

func listWorkProjects() ([]work.Project, error) {
	workProjectsStoreMu.Lock()
	defer workProjectsStoreMu.Unlock()
	state, _, err := loadWorkProjectsState()
	if err != nil {
		return nil, err
	}
	out := make([]work.Project, len(state.Projects))
	copy(out, state.Projects)
	return out, nil
}

func getWorkProject(projectID string) (work.Project, bool, error) {
	workProjectsStoreMu.Lock()
	defer workProjectsStoreMu.Unlock()
	state, _, err := loadWorkProjectsState()
	if err != nil {
		return work.Project{}, false, err
	}
	id := strings.TrimSpace(projectID)
	for _, project := range state.Projects {
		if strings.EqualFold(strings.TrimSpace(project.ID), id) {
			return project, true, nil
		}
	}
	return work.Project{}, false, nil
}

func upsertWorkProject(project work.Project) (work.Project, error) {
	workProjectsStoreMu.Lock()
	defer workProjectsStoreMu.Unlock()

	state, path, err := loadWorkProjectsState()
	if err != nil {
		return work.Project{}, err
	}
	normalized, err := work.NormalizeProject(project)
	if err != nil {
		return work.Project{}, err
	}
	if normalized.State == "" {
		normalized.State = work.ProjectStateRegistered
	}

	updated := false
	for i := range state.Projects {
		if !strings.EqualFold(strings.TrimSpace(state.Projects[i].ID), normalized.ID) {
			continue
		}
		if normalized.WorkflowDigest == "" {
			normalized.WorkflowDigest = state.Projects[i].WorkflowDigest
		}
		if normalized.LastSyncAt == "" {
			normalized.LastSyncAt = state.Projects[i].LastSyncAt
		}
		if normalized.LastSyncError == "" {
			normalized.LastSyncError = state.Projects[i].LastSyncError
		}
		if normalized.State == "" {
			normalized.State = state.Projects[i].State
		}
		state.Projects[i] = normalized
		updated = true
		break
	}
	if !updated {
		state.Projects = append(state.Projects, normalized)
	}
	if err := saveWorkProjectsState(path, state); err != nil {
		return work.Project{}, err
	}
	return normalized, nil
}

func syncWorkProject(projectID string) (work.Project, error) {
	workProjectsStoreMu.Lock()
	state, path, err := loadWorkProjectsState()
	if err != nil {
		workProjectsStoreMu.Unlock()
		return work.Project{}, err
	}
	id := strings.TrimSpace(projectID)
	index := -1
	project := work.Project{}
	for i := range state.Projects {
		if strings.EqualFold(strings.TrimSpace(state.Projects[i].ID), id) {
			index = i
			project = state.Projects[i]
			break
		}
	}
	if index == -1 {
		workProjectsStoreMu.Unlock()
		return work.Project{}, os.ErrNotExist
	}
	project.State = work.ProjectStateSyncing
	project.LastSyncError = ""
	state.Projects[index] = project
	if err := saveWorkProjectsState(path, state); err != nil {
		workProjectsStoreMu.Unlock()
		return work.Project{}, err
	}
	workProjectsStoreMu.Unlock()

	synced, _, syncErr := work.SyncProjectRepo(project)

	workProjectsStoreMu.Lock()
	defer workProjectsStoreMu.Unlock()
	state, path, err = loadWorkProjectsState()
	if err != nil {
		return work.Project{}, err
	}
	index = -1
	for i := range state.Projects {
		if strings.EqualFold(strings.TrimSpace(state.Projects[i].ID), id) {
			index = i
			break
		}
	}
	if index == -1 {
		return work.Project{}, os.ErrNotExist
	}
	if syncErr != nil {
		project = state.Projects[index]
		project.State = work.ProjectStateError
		project.LastSyncError = strings.TrimSpace(syncErr.Error())
		state.Projects[index] = project
		if err := saveWorkProjectsState(path, state); err != nil {
			return work.Project{}, err
		}
		return work.Project{}, syncErr
	}
	state.Projects[index] = synced
	if err := saveWorkProjectsState(path, state); err != nil {
		return work.Project{}, err
	}
	return synced, nil
}
