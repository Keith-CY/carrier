package gateway

import (
	"carrier/shared/work"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkItemStoreCreateAndList(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	item, err := upsertWorkItem(work.WorkItem{
		ProjectID:   "proj_123",
		Title:       "Add queue",
		Description: "Build work queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}
	if item.State != work.WorkItemStateNew {
		t.Fatalf("state=%q", item.State)
	}
	if item.CreatedAt == "" || item.UpdatedAt == "" {
		t.Fatalf("expected timestamps: %+v", item)
	}

	items, err := listWorkItems()
	if err != nil {
		t.Fatalf("listWorkItems error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len=%d", len(items))
	}
}

func TestWorkItemStorePersistsUnderProjectDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: "proj_123",
		Title:     "Add queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}

	path := filepath.Join(root, "works", "proj_123", "items", item.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted item: %v", err)
	}
}

func TestWorkItemStoreClaimAndRunTransitions(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: "proj_123",
		Title:     "Add queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}

	for _, next := range []work.WorkItemState{work.WorkItemStateTriaged, work.WorkItemStateQueued} {
		item, err = setWorkItemState(item.ID, next, "")
		if err != nil {
			t.Fatalf("setWorkItemState %s error: %v", next, err)
		}
	}
	item, err = setWorkItemState(item.ID, work.WorkItemStateClaimed, "run_123")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if item.ClaimedByRunID != "run_123" {
		t.Fatalf("claimedByRunId=%q", item.ClaimedByRunID)
	}
	item, err = setWorkItemState(item.ID, work.WorkItemStateRunning, "run_123")
	if err != nil {
		t.Fatalf("running error: %v", err)
	}
	if item.LatestRunID != "run_123" {
		t.Fatalf("latestRunId=%q", item.LatestRunID)
	}
}

func TestGetWorkItemUsesDirectLookup(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	if _, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: "proj_123",
		Title:     "Add queue",
	}); err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}

	item, ok, err := getWorkItem("work_123")
	if err != nil {
		t.Fatalf("getWorkItem error: %v", err)
	}
	if !ok {
		t.Fatal("expected work item lookup to succeed")
	}
	if item.ID != "work_123" {
		t.Fatalf("item.ID=%q", item.ID)
	}
}
