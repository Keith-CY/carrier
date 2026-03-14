package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"

	workmodel "carrier/shared/work"
)

type workCommandOptions struct {
	Action            string
	ProjectID         string
	WorkItemID        string
	RunID             string
	ClaimRunID        string
	Name              string
	SourceType        workmodel.SourceType
	SourceRef         string
	DefaultBranch     string
	WorkflowPath      string
	Title             string
	Description       string
	Acceptance        []string
	Priority          workmodel.WorkPriority
	Labels            []string
	Backend           workmodel.RunBackend
	Repository        string
	IssueNumber       int
	PullRequestNumber int
	PublishTargets    []string
	JSON              bool
}

type workProjectListCLIResponse struct {
	Projects []workmodel.Project `json:"projects"`
}

type workProjectCLIResponse struct {
	Project workmodel.Project `json:"project"`
}

type workItemListCLIResponse struct {
	Items []workmodel.WorkItem `json:"items"`
}

type workItemCLIResponse struct {
	Item workmodel.WorkItem `json:"item"`
}

type workRunListCLIResponse struct {
	Runs []workmodel.Run `json:"runs"`
}

type workRunCLIResponse struct {
	Run workmodel.Run `json:"run"`
}

type workGitHubPublishTargetCLIStatus struct {
	Target  string `json:"target"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

type workGitHubPublishRecordCLI struct {
	ID         string                             `json:"id"`
	ProjectID  string                             `json:"projectId"`
	WorkItemID string                             `json:"workItemId"`
	RunID      string                             `json:"runId"`
	Repository string                             `json:"repository,omitempty"`
	SourceRef  string                             `json:"sourceRef,omitempty"`
	Targets    []workGitHubPublishTargetCLIStatus `json:"targets"`
	CreatedAt  string                             `json:"createdAt"`
}

type workGitHubPublishCLIResponse struct {
	Record workGitHubPublishRecordCLI `json:"record"`
}

func parseWorkCommandArgs(args []string) (workCommandOptions, error) {
	if len(args) == 0 {
		return workCommandOptions{}, fmt.Errorf("usage: carrier work <projects|items|runs|github> ...")
	}
	subject := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	switch subject {
	case "projects":
		return parseWorkProjectsCommandArgs(rest)
	case "items":
		return parseWorkItemsCommandArgs(rest)
	case "runs":
		return parseWorkRunsCommandArgs(rest)
	case "github":
		return parseWorkGitHubCommandArgs(rest)
	default:
		return workCommandOptions{}, fmt.Errorf("unknown work subject: %s", args[0])
	}
}

func parseWorkProjectsCommandArgs(args []string) (workCommandOptions, error) {
	if len(args) == 0 {
		return workCommandOptions{Action: "projects-list"}, nil
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	opts := workCommandOptions{Action: "projects-" + action, SourceType: workmodel.SourceTypeLocal}
	switch action {
	case "list":
		return parseWorkJSONOnlyFlags(rest, opts)
	case "show", "sync", "archive":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("project id is required")
		}
		opts.ProjectID = strings.TrimSpace(rest[0])
		return parseWorkJSONOnlyFlags(rest[1:], opts)
	case "add":
		for i := 0; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--name":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Name = value
				i = next
			case "--source-type":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.SourceType = workmodel.SourceType(strings.ToLower(value))
				i = next
			case "--source-ref":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.SourceRef = value
				i = next
			case "--default-branch":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.DefaultBranch = value
				i = next
			case "--workflow-path":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.WorkflowPath = value
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work projects add option: %s", raw)
			}
		}
		if strings.TrimSpace(opts.Name) == "" {
			return workCommandOptions{}, fmt.Errorf("project name is required")
		}
		if strings.TrimSpace(opts.SourceRef) == "" {
			return workCommandOptions{}, fmt.Errorf("project source ref is required")
		}
		return opts, nil
	default:
		return workCommandOptions{}, fmt.Errorf("unsupported work projects action: %s", action)
	}
}

func parseWorkItemsCommandArgs(args []string) (workCommandOptions, error) {
	if len(args) == 0 {
		return workCommandOptions{Action: "items-list"}, nil
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	opts := workCommandOptions{Action: "items-" + action, Priority: workmodel.WorkPriorityNormal}
	switch action {
	case "list":
		return parseWorkJSONOnlyFlags(rest, opts)
	case "show", "cancel", "complete":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("work item id is required")
		}
		opts.WorkItemID = strings.TrimSpace(rest[0])
		return parseWorkJSONOnlyFlags(rest[1:], opts)
	case "claim":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("work item id is required")
		}
		opts.WorkItemID = strings.TrimSpace(rest[0])
		for i := 1; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--run-id":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.ClaimRunID = value
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work items claim option: %s", raw)
			}
		}
		if strings.TrimSpace(opts.ClaimRunID) == "" {
			return workCommandOptions{}, fmt.Errorf("claim requires --run-id")
		}
		return opts, nil
	case "create":
		for i := 0; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--project":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.ProjectID = value
				i = next
			case "--title":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Title = value
				i = next
			case "--description":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Description = value
				i = next
			case "--acceptance":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Acceptance = append(opts.Acceptance, value)
				i = next
			case "--priority":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Priority = workmodel.WorkPriority(strings.ToLower(value))
				i = next
			case "--label":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Labels = append(opts.Labels, value)
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work items create option: %s", raw)
			}
		}
		if strings.TrimSpace(opts.ProjectID) == "" {
			return workCommandOptions{}, fmt.Errorf("work item project is required")
		}
		if strings.TrimSpace(opts.Title) == "" {
			return workCommandOptions{}, fmt.Errorf("work item title is required")
		}
		return opts, nil
	case "update":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("work item id is required")
		}
		opts.WorkItemID = strings.TrimSpace(rest[0])
		for i := 1; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--title":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Title = value
				i = next
			case "--description":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Description = value
				i = next
			case "--acceptance":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Acceptance = append(opts.Acceptance, value)
				i = next
			case "--priority":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Priority = workmodel.WorkPriority(strings.ToLower(value))
				i = next
			case "--label":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Labels = append(opts.Labels, value)
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work items update option: %s", raw)
			}
		}
		if strings.TrimSpace(opts.Title) == "" && strings.TrimSpace(opts.Description) == "" && len(opts.Acceptance) == 0 && opts.Priority == workmodel.WorkPriorityNormal && len(opts.Labels) == 0 {
			return workCommandOptions{}, fmt.Errorf("work item update requires at least one field")
		}
		return opts, nil
	default:
		return workCommandOptions{}, fmt.Errorf("unsupported work items action: %s", action)
	}
}

func parseWorkRunsCommandArgs(args []string) (workCommandOptions, error) {
	if len(args) == 0 {
		return workCommandOptions{Action: "runs-list"}, nil
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	opts := workCommandOptions{Action: "runs-" + action, Backend: workmodel.RunBackendLocalSandboxed}
	switch action {
	case "list":
		return parseWorkJSONOnlyFlags(rest, opts)
	case "show", "resume", "cancel", "reclaim", "cleanup":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("run id is required")
		}
		opts.RunID = strings.TrimSpace(rest[0])
		return parseWorkJSONOnlyFlags(rest[1:], opts)
	case "start":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("work item id is required")
		}
		opts.WorkItemID = strings.TrimSpace(rest[0])
		for i := 1; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--backend":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Backend = workmodel.RunBackend(strings.ToLower(value))
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work runs start option: %s", raw)
			}
		}
		return opts, nil
	default:
		return workCommandOptions{}, fmt.Errorf("unsupported work runs action: %s", action)
	}
}

func parseWorkGitHubCommandArgs(args []string) (workCommandOptions, error) {
	if len(args) == 0 {
		return workCommandOptions{}, fmt.Errorf("usage: carrier work github <import|publish> ...")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	opts := workCommandOptions{Action: "github-" + action}
	switch action {
	case "import":
		for i := 0; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--project":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.ProjectID = value
				i = next
			case "--repository":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.Repository = value
				i = next
			case "--issue":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				num, convErr := strconv.Atoi(value)
				if convErr != nil || num <= 0 {
					return workCommandOptions{}, fmt.Errorf("issue number must be a positive integer")
				}
				opts.IssueNumber = num
				i = next
			case "--pr":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				num, convErr := strconv.Atoi(value)
				if convErr != nil || num <= 0 {
					return workCommandOptions{}, fmt.Errorf("pull request number must be a positive integer")
				}
				opts.PullRequestNumber = num
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work github import option: %s", raw)
			}
		}
		if strings.TrimSpace(opts.ProjectID) == "" {
			return workCommandOptions{}, fmt.Errorf("github import requires --project")
		}
		if strings.TrimSpace(opts.Repository) == "" {
			return workCommandOptions{}, fmt.Errorf("github import requires --repository")
		}
		if (opts.IssueNumber > 0 && opts.PullRequestNumber > 0) || (opts.IssueNumber <= 0 && opts.PullRequestNumber <= 0) {
			return workCommandOptions{}, fmt.Errorf("github import requires exactly one of --issue or --pr")
		}
		return opts, nil
	case "publish":
		if len(rest) == 0 || strings.HasPrefix(strings.TrimSpace(rest[0]), "-") {
			return workCommandOptions{}, fmt.Errorf("run id is required")
		}
		opts.RunID = strings.TrimSpace(rest[0])
		for i := 1; i < len(rest); i++ {
			raw := strings.TrimSpace(rest[i])
			switch raw {
			case "--target":
				value, next, err := requireNextValue(rest, i)
				if err != nil {
					return workCommandOptions{}, err
				}
				opts.PublishTargets = append(opts.PublishTargets, strings.ToLower(value))
				i = next
			case "--json":
				opts.JSON = true
			case "":
			default:
				return workCommandOptions{}, fmt.Errorf("unknown work github publish option: %s", raw)
			}
		}
		if len(opts.PublishTargets) == 0 {
			return workCommandOptions{}, fmt.Errorf("github publish requires at least one --target")
		}
		return opts, nil
	default:
		return workCommandOptions{}, fmt.Errorf("unsupported work github action: %s", action)
	}
}

func parseWorkJSONOnlyFlags(args []string, opts workCommandOptions) (workCommandOptions, error) {
	for _, raw := range args {
		switch strings.TrimSpace(raw) {
		case "", "--json":
			if strings.TrimSpace(raw) == "--json" {
				opts.JSON = true
			}
		default:
			return workCommandOptions{}, fmt.Errorf("unknown option: %s", raw)
		}
	}
	return opts, nil
}

func requireNextValue(args []string, index int) (string, int, error) {
	next := index + 1
	if next >= len(args) {
		return "", index, fmt.Errorf("missing value for %s", strings.TrimSpace(args[index]))
	}
	value := strings.TrimSpace(args[next])
	if value == "" || strings.HasPrefix(value, "--") {
		return "", index, fmt.Errorf("missing value for %s", strings.TrimSpace(args[index]))
	}
	return value, next, nil
}

func runWorkCommand(out io.Writer, opts workCommandOptions) error {
	switch opts.Action {
	case "projects-list":
		resp, raw, err := listWorkProjectsCLI()
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkProjectList(resp.Projects))
		return nil
	case "projects-show":
		resp, raw, err := fetchWorkProjectCLI(opts.ProjectID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkProject(resp.Project))
		return nil
	case "projects-sync":
		resp, raw, err := syncWorkProjectCLI(opts.ProjectID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkProject(resp.Project))
		return nil
	case "projects-add":
		resp, raw, err := createWorkProjectCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkProject(resp.Project))
		return nil
	case "items-list":
		resp, raw, err := listWorkItemsCLI()
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItemList(resp.Items))
		return nil
	case "items-show":
		resp, raw, err := fetchWorkItemCLI(opts.WorkItemID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "items-create":
		resp, raw, err := createWorkItemCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "items-update":
		resp, raw, err := updateWorkItemCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "items-claim":
		resp, raw, err := claimWorkItemCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "items-cancel":
		resp, raw, err := cancelWorkItemCLI(opts.WorkItemID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "items-complete":
		resp, raw, err := completeWorkItemCLI(opts.WorkItemID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "runs-list":
		resp, raw, err := listWorkRunsCLI()
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkRunList(resp.Runs))
		return nil
	case "runs-show":
		resp, raw, err := fetchWorkRunCLI(opts.RunID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkRun(resp.Run))
		return nil
	case "runs-start":
		resp, raw, err := startWorkRunCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkRun(resp.Run))
		return nil
	case "runs-resume", "runs-cancel", "runs-reclaim", "runs-cleanup":
		resp, raw, err := mutateWorkRunCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		if opts.Action == "runs-cleanup" {
			_, _ = fmt.Fprintf(out, "cleaned workspace for run %s\n", strings.TrimSpace(opts.RunID))
			return nil
		}
		_, _ = fmt.Fprintln(out, renderWorkRun(resp.Run))
		return nil
	case "github-import":
		resp, raw, err := importGitHubWorkItemCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderWorkItem(resp.Item))
		return nil
	case "github-publish":
		resp, raw, err := publishGitHubWorkRunCLI(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderGitHubPublishRecord(resp.Record))
		return nil
	default:
		return fmt.Errorf("unsupported work action: %s", opts.Action)
	}
}

func listWorkProjectsCLI() (*workProjectListCLIResponse, []byte, error) {
	raw, _, err := gatewayRequest(http.MethodGet, "/api/v1/work/projects", nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workProjectListCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work projects response: %w", err)
	}
	return &resp, raw, nil
}

func fetchWorkProjectCLI(projectID string) (*workProjectCLIResponse, []byte, error) {
	path := "/api/v1/work/projects/" + neturl.PathEscape(strings.TrimSpace(projectID))
	raw, _, err := gatewayRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workProjectCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work project response: %w", err)
	}
	return &resp, raw, nil
}

func syncWorkProjectCLI(projectID string) (*workProjectCLIResponse, []byte, error) {
	path := "/api/v1/work/projects/" + neturl.PathEscape(strings.TrimSpace(projectID)) + "/sync"
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, nil, err
	}
	var resp workProjectCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work project sync response: %w", err)
	}
	return &resp, raw, nil
}

func createWorkProjectCLI(opts workCommandOptions) (*workProjectCLIResponse, []byte, error) {
	payload := map[string]any{
		"name":       strings.TrimSpace(opts.Name),
		"sourceType": opts.SourceType,
		"sourceRef":  strings.TrimSpace(opts.SourceRef),
	}
	if trimmed := strings.TrimSpace(opts.DefaultBranch); trimmed != "" {
		payload["defaultBranch"] = trimmed
	}
	if trimmed := strings.TrimSpace(opts.WorkflowPath); trimmed != "" {
		payload["workflowPath"] = trimmed
	}
	raw, _, err := gatewayRequest(http.MethodPost, "/api/v1/work/projects", payload)
	if err != nil {
		return nil, nil, err
	}
	var resp workProjectCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work project create response: %w", err)
	}
	return &resp, raw, nil
}

func listWorkItemsCLI() (*workItemListCLIResponse, []byte, error) {
	raw, _, err := gatewayRequest(http.MethodGet, "/api/v1/work/items", nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workItemListCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work items response: %w", err)
	}
	return &resp, raw, nil
}

func fetchWorkItemCLI(itemID string) (*workItemCLIResponse, []byte, error) {
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(itemID))
	raw, _, err := gatewayRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item response: %w", err)
	}
	return &resp, raw, nil
}

func createWorkItemCLI(opts workCommandOptions) (*workItemCLIResponse, []byte, error) {
	payload := map[string]any{
		"projectId":   strings.TrimSpace(opts.ProjectID),
		"title":       strings.TrimSpace(opts.Title),
		"description": strings.TrimSpace(opts.Description),
		"acceptance":  opts.Acceptance,
		"priority":    opts.Priority,
		"labels":      opts.Labels,
	}
	raw, _, err := gatewayRequest(http.MethodPost, "/api/v1/work/items", payload)
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item create response: %w", err)
	}
	return &resp, raw, nil
}

func updateWorkItemCLI(opts workCommandOptions) (*workItemCLIResponse, []byte, error) {
	payload := map[string]any{}
	if trimmed := strings.TrimSpace(opts.Title); trimmed != "" {
		payload["title"] = trimmed
	}
	if trimmed := strings.TrimSpace(opts.Description); trimmed != "" {
		payload["description"] = trimmed
	}
	if len(opts.Acceptance) > 0 {
		payload["acceptance"] = opts.Acceptance
	}
	if opts.Priority != workmodel.WorkPriorityNormal {
		payload["priority"] = opts.Priority
	}
	if len(opts.Labels) > 0 {
		payload["labels"] = opts.Labels
	}
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(opts.WorkItemID))
	raw, _, err := gatewayRequest(http.MethodPatch, path, payload)
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item update response: %w", err)
	}
	return &resp, raw, nil
}

func claimWorkItemCLI(opts workCommandOptions) (*workItemCLIResponse, []byte, error) {
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(opts.WorkItemID)) + "/claim"
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{"runId": strings.TrimSpace(opts.ClaimRunID)})
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item claim response: %w", err)
	}
	return &resp, raw, nil
}

func cancelWorkItemCLI(itemID string) (*workItemCLIResponse, []byte, error) {
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(itemID)) + "/cancel"
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item cancel response: %w", err)
	}
	return &resp, raw, nil
}

func completeWorkItemCLI(itemID string) (*workItemCLIResponse, []byte, error) {
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(itemID)) + "/complete"
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work item complete response: %w", err)
	}
	return &resp, raw, nil
}

func listWorkRunsCLI() (*workRunListCLIResponse, []byte, error) {
	raw, _, err := gatewayRequest(http.MethodGet, "/api/v1/work/runs", nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workRunListCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work runs response: %w", err)
	}
	return &resp, raw, nil
}

func fetchWorkRunCLI(runID string) (*workRunCLIResponse, []byte, error) {
	path := "/api/v1/work/runs/" + neturl.PathEscape(strings.TrimSpace(runID))
	raw, _, err := gatewayRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var resp workRunCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work run response: %w", err)
	}
	return &resp, raw, nil
}

func startWorkRunCLI(opts workCommandOptions) (*workRunCLIResponse, []byte, error) {
	path := "/api/v1/work/items/" + neturl.PathEscape(strings.TrimSpace(opts.WorkItemID)) + "/runs"
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{"backend": opts.Backend})
	if err != nil {
		return nil, nil, err
	}
	var resp workRunCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work run start response: %w", err)
	}
	return &resp, raw, nil
}

func mutateWorkRunCLI(opts workCommandOptions) (*workRunCLIResponse, []byte, error) {
	action := strings.TrimPrefix(opts.Action, "runs-")
	path := "/api/v1/work/runs/" + neturl.PathEscape(strings.TrimSpace(opts.RunID)) + "/" + neturl.PathEscape(action)
	raw, _, err := gatewayRequest(http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, nil, err
	}
	if action == "cleanup" {
		return &workRunCLIResponse{}, raw, nil
	}
	var resp workRunCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode work run mutation response: %w", err)
	}
	return &resp, raw, nil
}

func importGitHubWorkItemCLI(opts workCommandOptions) (*workItemCLIResponse, []byte, error) {
	payload := map[string]any{
		"projectId":  strings.TrimSpace(opts.ProjectID),
		"repository": strings.TrimSpace(opts.Repository),
	}
	if opts.IssueNumber > 0 {
		payload["issueNumber"] = opts.IssueNumber
	}
	if opts.PullRequestNumber > 0 {
		payload["pullRequestNumber"] = opts.PullRequestNumber
	}
	raw, _, err := gatewayRequest(http.MethodPost, "/api/v1/work/adapters/github/import", payload)
	if err != nil {
		return nil, nil, err
	}
	var resp workItemCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode github import response: %w", err)
	}
	return &resp, raw, nil
}

func publishGitHubWorkRunCLI(opts workCommandOptions) (*workGitHubPublishCLIResponse, []byte, error) {
	payload := map[string]any{
		"runId":   strings.TrimSpace(opts.RunID),
		"targets": opts.PublishTargets,
	}
	raw, _, err := gatewayRequest(http.MethodPost, "/api/v1/work/adapters/github/publish", payload)
	if err != nil {
		return nil, nil, err
	}
	var resp workGitHubPublishCLIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("decode github publish response: %w", err)
	}
	return &resp, raw, nil
}

func renderWorkProjectList(projects []workmodel.Project) string {
	if len(projects) == 0 {
		return "no work projects"
	}
	sorted := append([]workmodel.Project(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.TrimSpace(sorted[i].Name) < strings.TrimSpace(sorted[j].Name)
	})
	lines := make([]string, 0, len(sorted))
	for _, project := range sorted {
		lines = append(lines, fmt.Sprintf("%s state=%s source=%s", strings.TrimSpace(project.ID), strings.TrimSpace(string(project.State)), strings.TrimSpace(project.SourceRef)))
	}
	return strings.Join(lines, "\n")
}

func renderWorkProject(project workmodel.Project) string {
	lines := []string{
		fmt.Sprintf("project=%s", strings.TrimSpace(project.ID)),
		fmt.Sprintf("name=%s", strings.TrimSpace(project.Name)),
		fmt.Sprintf("state=%s", strings.TrimSpace(string(project.State))),
		fmt.Sprintf("source=%s", strings.TrimSpace(project.SourceRef)),
	}
	if trimmed := strings.TrimSpace(project.WorkflowDigest); trimmed != "" {
		lines = append(lines, fmt.Sprintf("workflow=%s", trimmed))
	}
	return strings.Join(lines, "\n")
}

func renderWorkItemList(items []workmodel.WorkItem) string {
	if len(items) == 0 {
		return "no work items"
	}
	sorted := append([]workmodel.WorkItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.TrimSpace(sorted[i].ID) < strings.TrimSpace(sorted[j].ID)
	})
	lines := make([]string, 0, len(sorted))
	for _, item := range sorted {
		lines = append(lines, fmt.Sprintf("%s [%s] %s", strings.TrimSpace(item.ID), strings.TrimSpace(string(item.State)), strings.TrimSpace(item.Title)))
	}
	return strings.Join(lines, "\n")
}

func renderWorkItem(item workmodel.WorkItem) string {
	lines := []string{
		fmt.Sprintf("work_item=%s", strings.TrimSpace(item.ID)),
		fmt.Sprintf("project=%s", strings.TrimSpace(item.ProjectID)),
		fmt.Sprintf("state=%s", strings.TrimSpace(string(item.State))),
		fmt.Sprintf("priority=%s", strings.TrimSpace(string(item.Priority))),
		fmt.Sprintf("title=%s", strings.TrimSpace(item.Title)),
	}
	if trimmed := strings.TrimSpace(item.SourceRef); trimmed != "" {
		lines = append(lines, fmt.Sprintf("source=%s", trimmed))
	}
	if trimmed := strings.TrimSpace(item.LatestRunID); trimmed != "" {
		lines = append(lines, fmt.Sprintf("latest_run=%s", trimmed))
	}
	if len(item.Acceptance) > 0 {
		lines = append(lines, fmt.Sprintf("acceptance=%s", strings.Join(item.Acceptance, " | ")))
	}
	return strings.Join(lines, "\n")
}

func renderWorkRunList(runs []workmodel.Run) string {
	if len(runs) == 0 {
		return "no work runs"
	}
	sorted := append([]workmodel.Run(nil), runs...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.TrimSpace(sorted[i].ID) < strings.TrimSpace(sorted[j].ID)
	})
	lines := make([]string, 0, len(sorted))
	for _, run := range sorted {
		lines = append(lines, fmt.Sprintf("%s [%s] work=%s execution=%s", strings.TrimSpace(run.ID), strings.TrimSpace(string(run.Phase)), strings.TrimSpace(run.WorkItemID), strings.TrimSpace(run.ExecutionID)))
	}
	return strings.Join(lines, "\n")
}

func renderWorkRun(run workmodel.Run) string {
	lines := []string{
		fmt.Sprintf("run=%s", strings.TrimSpace(run.ID)),
		fmt.Sprintf("work_item=%s", strings.TrimSpace(run.WorkItemID)),
		fmt.Sprintf("phase=%s", strings.TrimSpace(string(run.Phase))),
		fmt.Sprintf("backend=%s", strings.TrimSpace(string(run.Backend))),
	}
	if trimmed := strings.TrimSpace(run.ExecutionID); trimmed != "" {
		lines = append(lines, fmt.Sprintf("execution=%s", trimmed))
	}
	if trimmed := strings.TrimSpace(run.WorkspacePath); trimmed != "" {
		lines = append(lines, fmt.Sprintf("workspace=%s", trimmed))
	}
	if trimmed := strings.TrimSpace(run.WorkflowDigest); trimmed != "" {
		lines = append(lines, fmt.Sprintf("workflow=%s", trimmed))
	}
	return strings.Join(lines, "\n")
}

func renderGitHubPublishRecord(record workGitHubPublishRecordCLI) string {
	lines := []string{
		fmt.Sprintf("publish=%s", strings.TrimSpace(record.ID)),
		fmt.Sprintf("run=%s", strings.TrimSpace(record.RunID)),
	}
	if trimmed := strings.TrimSpace(record.Repository); trimmed != "" {
		lines = append(lines, fmt.Sprintf("repository=%s", trimmed))
	}
	for _, target := range record.Targets {
		line := fmt.Sprintf("target[%s]=%s", strings.TrimSpace(target.Target), strings.TrimSpace(target.Status))
		if trimmed := strings.TrimSpace(target.Message); trimmed != "" {
			line += " (" + trimmed + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
