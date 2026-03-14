package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type workGitHubImportRequest struct {
	ProjectID         string   `json:"projectId"`
	Repository        string   `json:"repository"`
	IssueNumber       int      `json:"issueNumber,omitempty"`
	PullRequestNumber int      `json:"pullRequestNumber,omitempty"`
	Title             string   `json:"title,omitempty"`
	Body              string   `json:"body,omitempty"`
	Labels            []string `json:"labels,omitempty"`
}

type workGitHubIssueSnapshot struct {
	Title  string
	Body   string
	Labels []string
}

type workGitHubPublishRequest struct {
	RunID   string   `json:"runId"`
	Targets []string `json:"targets"`
}

type workGitHubPublishTargetStatus struct {
	Target  string `json:"target"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

type workGitHubPublishRecord struct {
	ID         string                          `json:"id"`
	ProjectID  string                          `json:"projectId"`
	WorkItemID string                          `json:"workItemId"`
	RunID      string                          `json:"runId"`
	Repository string                          `json:"repository,omitempty"`
	SourceRef  string                          `json:"sourceRef,omitempty"`
	Targets    []workGitHubPublishTargetStatus `json:"targets"`
	CreatedAt  string                          `json:"createdAt"`
}

var runWorkGitHubCommand = func(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func importGitHubWorkItem(req workGitHubImportRequest) (work.WorkItem, error) {
	project, ok, err := getWorkProject(strings.TrimSpace(req.ProjectID))
	if err != nil {
		return work.WorkItem{}, err
	}
	if !ok {
		return work.WorkItem{}, os.ErrNotExist
	}
	repository := strings.TrimSpace(req.Repository)
	if repository == "" {
		return work.WorkItem{}, fmt.Errorf("repository is required")
	}
	if (req.IssueNumber > 0 && req.PullRequestNumber > 0) || (req.IssueNumber <= 0 && req.PullRequestNumber <= 0) {
		return work.WorkItem{}, fmt.Errorf("exactly one of issueNumber or pullRequestNumber is required")
	}
	snapshot := workGitHubIssueSnapshot{
		Title:  strings.TrimSpace(req.Title),
		Body:   strings.TrimSpace(req.Body),
		Labels: normalizeWorkGitHubStringList(req.Labels),
	}
	number := req.IssueNumber
	sourceRef := fmt.Sprintf("github:%s/issues/%d", repository, number)
	if req.PullRequestNumber > 0 {
		number = req.PullRequestNumber
		sourceRef = fmt.Sprintf("github:%s/pull/%d", repository, number)
	}
	if snapshot.Title == "" {
		fetched, err := fetchGitHubIssueSnapshot(repository, number)
		if err != nil {
			return work.WorkItem{}, err
		}
		snapshot = fetched
	}
	item, err := upsertWorkItem(work.WorkItem{
		ProjectID:   project.ID,
		Title:       snapshot.Title,
		Description: snapshot.Body,
		Labels:      snapshot.Labels,
		Priority:    work.WorkPriorityNormal,
		Source:      work.WorkSourceGitHub,
		SourceRef:   sourceRef,
	})
	if err != nil {
		return work.WorkItem{}, err
	}
	return item, nil
}

func fetchGitHubIssueSnapshot(repository string, number int) (workGitHubIssueSnapshot, error) {
	out, err := runWorkGitHubCommand("api", fmt.Sprintf("repos/%s/issues/%d", strings.TrimSpace(repository), number))
	if err != nil {
		return workGitHubIssueSnapshot{}, err
	}
	var payload struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return workGitHubIssueSnapshot{}, fmt.Errorf("decode github issue response: %w", err)
	}
	labels := make([]string, 0, len(payload.Labels))
	for _, label := range payload.Labels {
		if trimmed := strings.TrimSpace(label.Name); trimmed != "" {
			labels = append(labels, trimmed)
		}
	}
	return workGitHubIssueSnapshot{Title: strings.TrimSpace(payload.Title), Body: strings.TrimSpace(payload.Body), Labels: normalizeWorkGitHubStringList(labels)}, nil
}

func publishGitHubRun(req workGitHubPublishRequest) (workGitHubPublishRecord, error) {
	run, ok, err := getWorkRun(strings.TrimSpace(req.RunID))
	if err != nil {
		return workGitHubPublishRecord{}, err
	}
	if !ok {
		return workGitHubPublishRecord{}, os.ErrNotExist
	}
	item, ok, err := getWorkItem(run.WorkItemID)
	if err != nil {
		return workGitHubPublishRecord{}, err
	}
	if !ok {
		return workGitHubPublishRecord{}, os.ErrNotExist
	}
	repository, number, ok := parseGitHubIssueReference(item.SourceRef)
	statuses := make([]workGitHubPublishTargetStatus, 0, len(req.Targets))
	normalizedTargets := normalizeWorkGitHubStringList(req.Targets)
	sort.Strings(normalizedTargets)
	for _, target := range normalizedTargets {
		status := workGitHubPublishTargetStatus{Target: target}
		switch target {
		case "comment", "status_note":
			if !ok {
				status.Status = string(work.PublishStatusSkipped)
				status.Message = "work item is not linked to a github issue or pull request"
				break
			}
			body := renderGitHubPublishComment(item, run, target)
			if err := postGitHubIssueComment(repository, number, body); err != nil {
				status.Status = string(work.PublishStatusFailed)
				status.Message = err.Error()
			} else {
				status.Status = string(work.PublishStatusPublished)
			}
		case "branch", "pr_draft":
			status.Status = string(work.PublishStatusSkipped)
			status.Message = "target is reserved for a later phase"
		default:
			status.Status = string(work.PublishStatusFailed)
			status.Message = "unsupported target"
		}
		statuses = append(statuses, status)
	}
	record := workGitHubPublishRecord{
		ID:         "publish_" + uuid.NewString(),
		ProjectID:  run.ProjectID,
		WorkItemID: run.WorkItemID,
		RunID:      run.ID,
		Repository: repository,
		SourceRef:  strings.TrimSpace(item.SourceRef),
		Targets:    statuses,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveGitHubPublishRecord(record); err != nil {
		return workGitHubPublishRecord{}, err
	}
	run.PublishStatus = aggregateGitHubPublishStatus(statuses)
	if run.PublishStatus == work.PublishStatusFailed {
		run.Phase = work.RunPhaseFailed
	} else {
		run.Phase = work.RunPhaseCompleted
	}
	run.UpdatedAt = nowTimestamp()
	if err := saveWorkRun(run); err != nil {
		return workGitHubPublishRecord{}, err
	}
	if run.PublishStatus == work.PublishStatusPublished {
		if _, err := setWorkItemState(item.ID, work.WorkItemStateAwaitingReview, run.ID); err != nil {
			return workGitHubPublishRecord{}, err
		}
	}
	return record, nil
}

func renderGitHubPublishComment(item work.WorkItem, run work.Run, target string) string {
	lines := []string{
		fmt.Sprintf("Carrier work update for %s", strings.TrimSpace(item.ID)),
		"",
		fmt.Sprintf("- title: %s", strings.TrimSpace(item.Title)),
		fmt.Sprintf("- run: %s", strings.TrimSpace(run.ID)),
		fmt.Sprintf("- phase: %s", strings.TrimSpace(string(run.Phase))),
		fmt.Sprintf("- execution: %s", strings.TrimSpace(run.ExecutionID)),
		fmt.Sprintf("- verification: %s", strings.TrimSpace(string(run.VerificationStatus))),
		fmt.Sprintf("- publish target: %s", strings.TrimSpace(target)),
	}
	return strings.Join(lines, "\n")
}

func postGitHubIssueComment(repository string, number int, body string) error {
	_, err := runWorkGitHubCommand(
		"api",
		fmt.Sprintf("repos/%s/issues/%d/comments", strings.TrimSpace(repository), number),
		"-f", "body="+body,
	)
	return err
}

func parseGitHubIssueReference(sourceRef string) (string, int, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sourceRef), "github:"))
	if trimmed == "" {
		return "", 0, false
	}
	if repository, number, ok := strings.Cut(trimmed, "/issues/"); ok {
		issueNumber, err := strconv.Atoi(strings.TrimSpace(number))
		if err == nil && strings.TrimSpace(repository) != "" && issueNumber > 0 {
			return strings.TrimSpace(repository), issueNumber, true
		}
	}
	if repository, number, ok := strings.Cut(trimmed, "/pull/"); ok {
		issueNumber, err := strconv.Atoi(strings.TrimSpace(number))
		if err == nil && strings.TrimSpace(repository) != "" && issueNumber > 0 {
			return strings.TrimSpace(repository), issueNumber, true
		}
	}
	return "", 0, false
}

func aggregateGitHubPublishStatus(statuses []workGitHubPublishTargetStatus) work.PublishStatus {
	if len(statuses) == 0 {
		return work.PublishStatusSkipped
	}
	hasPublished := false
	hasSkipped := false
	for _, status := range statuses {
		switch strings.TrimSpace(status.Status) {
		case string(work.PublishStatusFailed):
			return work.PublishStatusFailed
		case string(work.PublishStatusPublished):
			hasPublished = true
		case string(work.PublishStatusSkipped):
			hasSkipped = true
		}
	}
	if hasPublished {
		return work.PublishStatusPublished
	}
	if hasSkipped {
		return work.PublishStatusSkipped
	}
	return work.PublishStatusPending
}

func saveGitHubPublishRecord(record workGitHubPublishRecord) error {
	roots, err := work.ResolveRoots()
	if err != nil {
		return err
	}
	path := filepath.Join(roots.Works, strings.TrimSpace(record.ProjectID), "publishes", strings.TrimSpace(record.ID)+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create publish dir: %w", err)
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal github publish record: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write github publish record: %w", err)
	}
	return nil
}

func listGitHubPublishRecords(projectID, runID string) ([]workGitHubPublishRecord, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || runID == "" {
		return nil, nil
	}
	roots, err := work.ResolveRoots()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(roots.Works, projectID, "publishes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read publish dir: %w", err)
	}
	records := make([]workGitHubPublishRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read publish record: %w", err)
		}
		var record workGitHubPublishRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("parse publish record: %w", err)
		}
		if strings.TrimSpace(record.RunID) != runID {
			continue
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt != records[j].CreatedAt {
			return records[i].CreatedAt < records[j].CreatedAt
		}
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func normalizeWorkGitHubStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
