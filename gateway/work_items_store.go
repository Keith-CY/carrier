package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var workItemsStoreMu sync.Mutex

func listWorkItems() ([]work.WorkItem, error) {
	workItemsStoreMu.Lock()
	defer workItemsStoreMu.Unlock()

	roots, err := work.ResolveRoots()
	if err != nil {
		return nil, err
	}
	projectEntries, err := os.ReadDir(roots.Works)
	if err != nil {
		if os.IsNotExist(err) {
			return []work.WorkItem{}, nil
		}
		return nil, fmt.Errorf("read works root: %w", err)
	}
	items := make([]work.WorkItem, 0)
	for _, entry := range projectEntries {
		if !entry.IsDir() {
			continue
		}
		itemDir := filepath.Join(roots.Works, entry.Name(), "items")
		fileEntries, readErr := os.ReadDir(itemDir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("read work items dir: %w", readErr)
		}
		for _, fileEntry := range fileEntries {
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".json") {
				continue
			}
			item, err := loadWorkItemFromPath(filepath.Join(itemDir, fileEntry.Name()))
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].UpdatedAt) > strings.TrimSpace(items[j].UpdatedAt)
	})
	return items, nil
}

func getWorkItem(itemID string) (work.WorkItem, bool, error) {
	workItemsStoreMu.Lock()
	defer workItemsStoreMu.Unlock()

	item, _, err := loadWorkItemByID(itemID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return work.WorkItem{}, false, nil
		}
		return work.WorkItem{}, false, err
	}
	return item, true, nil
}

func upsertWorkItem(item work.WorkItem) (work.WorkItem, error) {
	workItemsStoreMu.Lock()
	defer workItemsStoreMu.Unlock()

	normalized, err := work.NormalizeWorkItem(item)
	if err != nil {
		return work.WorkItem{}, err
	}
	path, err := workItemPath(normalized.ProjectID, normalized.ID)
	if err != nil {
		return work.WorkItem{}, err
	}
	if existing, readErr := loadWorkItemIfExists(path); readErr != nil {
		return work.WorkItem{}, readErr
	} else if existing != nil {
		if normalized.CreatedAt == "" {
			normalized.CreatedAt = existing.CreatedAt
		}
		if normalized.LatestRunID == "" {
			normalized.LatestRunID = existing.LatestRunID
		}
		if normalized.ClaimedByRunID == "" {
			normalized.ClaimedByRunID = existing.ClaimedByRunID
		}
	}
	now := nowTimestamp()
	if normalized.CreatedAt == "" {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now
	if err := saveWorkItem(path, normalized); err != nil {
		return work.WorkItem{}, err
	}
	return normalized, nil
}

func setWorkItemState(itemID string, next work.WorkItemState, runID string) (work.WorkItem, error) {
	workItemsStoreMu.Lock()
	defer workItemsStoreMu.Unlock()

	item, path, err := loadWorkItemByID(itemID)
	if err != nil {
		return work.WorkItem{}, err
	}
	updated, err := transitionWorkItemState(item, next, runID)
	if err != nil {
		return work.WorkItem{}, err
	}
	updated.UpdatedAt = nowTimestamp()
	if err := saveWorkItem(path, updated); err != nil {
		return work.WorkItem{}, err
	}
	return updated, nil
}

func workItemPath(projectID, itemID string) (string, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return "", err
	}
	projectID, err = work.NormalizeProjectID(projectID)
	if err != nil {
		return "", err
	}
	itemID, err = work.NormalizeWorkItemID(itemID)
	if err != nil {
		return "", err
	}
	return filepath.Join(roots.Works, projectID, "items", itemID+".json"), nil
}

func loadWorkItemByID(itemID string) (work.WorkItem, string, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return work.WorkItem{}, "", err
	}
	projectEntries, err := os.ReadDir(roots.Works)
	if err != nil {
		if os.IsNotExist(err) {
			return work.WorkItem{}, "", os.ErrNotExist
		}
		return work.WorkItem{}, "", fmt.Errorf("read works root: %w", err)
	}
	id, err := work.NormalizeWorkItemID(itemID)
	if err != nil {
		return work.WorkItem{}, "", err
	}
	for _, entry := range projectEntries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(roots.Works, entry.Name(), "items", id+".json")
		item, err := loadWorkItemIfExists(path)
		if err != nil {
			return work.WorkItem{}, "", err
		}
		if item != nil {
			return *item, path, nil
		}
	}
	return work.WorkItem{}, "", os.ErrNotExist
}

func loadWorkItemFromPath(path string) (work.WorkItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return work.WorkItem{}, fmt.Errorf("read work item: %w", err)
	}
	var item work.WorkItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return work.WorkItem{}, fmt.Errorf("parse work item: %w", err)
	}
	return work.NormalizeWorkItem(item)
}

func loadWorkItemIfExists(path string) (*work.WorkItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read work item: %w", err)
	}
	var item work.WorkItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("parse work item: %w", err)
	}
	normalized, err := work.NormalizeWorkItem(item)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func saveWorkItem(path string, item work.WorkItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create work item dir: %w", err)
	}
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal work item: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write work item: %w", err)
	}
	return nil
}
