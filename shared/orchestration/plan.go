package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	LocalHostID              = "local"
	DefaultMaxConcurrency    = 8
	defaultApprovalScopeName = "infrastructure_only"
)

type DecomposeTask struct {
	ID      string `json:"id"`
	Input   string `json:"input"`
	AgentID string `json:"agentId,omitempty"`
}

type RequiredWorker struct {
	HostID     string   `json:"hostId,omitempty"`
	HostLabels []string `json:"hostLabels,omitempty"`
	AgentID    string   `json:"agentId"`
	Count      int      `json:"count"`
}

type TaskUnit struct {
	ID          string   `json:"id"`
	Input       string   `json:"input"`
	TimeoutMs   int      `json:"timeoutMs,omitempty"`
	RetryBudget int      `json:"retryBudget,omitempty"`
	HostID      string   `json:"hostId,omitempty"`
	HostLabels  []string `json:"hostLabels,omitempty"`
	AgentID     string   `json:"agentId,omitempty"`
}

type Plan struct {
	Goal            string           `json:"goal"`
	TemplateID      string           `json:"templateId,omitempty"`
	Provider        string           `json:"provider,omitempty"`
	HostIDs         []string         `json:"hostIds,omitempty"`
	HostLabels      []string         `json:"hostLabels,omitempty"`
	ApprovalScope   string           `json:"approvalScope"`
	MaxConcurrency  int              `json:"maxConcurrency"`
	PlannerTasks    []DecomposeTask  `json:"plannerTasks"`
	RequiredWorkers []RequiredWorker `json:"requiredWorkers"`
	TaskUnits       []TaskUnit       `json:"taskUnits"`
}

type BuildPlanInput struct {
	Goal           string
	TemplateID     string
	Provider       string
	HostIDs        []string
	HostLabels     []string
	MaxConcurrency int
	Tasks          []DecomposeTask
}

func BuildPlan(input BuildPlanInput) (Plan, error) {
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		return Plan{}, errors.New("goal is required")
	}

	plannerTasks := normalizePlannerTasks(input.Tasks)
	if len(plannerTasks) == 0 {
		plannerTasks = FallbackTasks(goal)
	}
	hostIDs := dedupeStrings(input.HostIDs)
	hostLabels := normalizeSelectorStrings(input.HostLabels)
	taskUnits := AssignTaskUnits(plannerTasks, hostIDs, hostLabels)
	requiredWorkers := BuildRequiredWorkers(taskUnits)
	if len(taskUnits) == 0 || len(requiredWorkers) == 0 {
		return Plan{}, errors.New("failed to build orchestrator task plan")
	}

	return Plan{
		Goal:            goal,
		TemplateID:      strings.TrimSpace(input.TemplateID),
		Provider:        strings.TrimSpace(input.Provider),
		HostIDs:         hostIDs,
		HostLabels:      hostLabels,
		ApprovalScope:   defaultApprovalScopeName,
		MaxConcurrency:  EffectiveMaxConcurrency(len(taskUnits), input.MaxConcurrency),
		PlannerTasks:    plannerTasks,
		RequiredWorkers: requiredWorkers,
		TaskUnits:       taskUnits,
	}, nil
}

func FallbackTasks(goal string) []DecomposeTask {
	return []DecomposeTask{{
		ID:      "task-1",
		Input:   strings.TrimSpace(goal),
		AgentID: "zeroclaw",
	}}
}

func AssignTaskUnits(tasks []DecomposeTask, hostIDs []string, hostLabels []string) []TaskUnit {
	trimmedHosts := dedupeStrings(hostIDs)
	trimmedHostLabels := normalizeSelectorStrings(hostLabels)
	taskHostLabels := trimmedHostLabels
	if len(trimmedHosts) == 0 {
		if len(trimmedHostLabels) == 0 {
			trimmedHosts = []string{LocalHostID}
		}
	} else {
		taskHostLabels = nil
	}

	agentOffsets := map[string]int{}
	out := make([]TaskUnit, 0, len(tasks))
	for idx, task := range normalizePlannerTasks(tasks) {
		agentID := NormalizeAgentID(task.AgentID)
		hostID := ""
		if len(trimmedHosts) > 0 {
			offset := agentOffsets[agentID]
			hostID = trimmedHosts[offset%len(trimmedHosts)]
			agentOffsets[agentID] = offset + 1
		}

		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = fmt.Sprintf("task-%d", idx+1)
		}
		out = append(out, TaskUnit{
			ID:          taskID,
			Input:       strings.TrimSpace(task.Input),
			TimeoutMs:   60_000,
			RetryBudget: 0,
			HostID:      hostID,
			HostLabels:  append([]string(nil), taskHostLabels...),
			AgentID:     agentID,
		})
	}
	return out
}

func BuildRequiredWorkers(taskUnits []TaskUnit) []RequiredWorker {
	type workerKey struct {
		hostID  string
		labelID string
		agentID string
	}
	seen := map[workerKey]struct{}{}
	workers := make([]RequiredWorker, 0, len(taskUnits))
	for _, task := range taskUnits {
		hostID := strings.TrimSpace(task.HostID)
		hostLabels := normalizeSelectorStrings(task.HostLabels)
		if hostID == "" {
			if len(hostLabels) == 0 {
				hostID = LocalHostID
			}
		}
		agentID := NormalizeAgentID(task.AgentID)
		key := workerKey{hostID: hostID, labelID: strings.Join(hostLabels, ","), agentID: agentID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		workers = append(workers, RequiredWorker{
			HostID:     hostID,
			HostLabels: hostLabels,
			AgentID:    agentID,
			Count:      1,
		})
	}
	sort.Slice(workers, func(i, j int) bool {
		leftHost := workers[i].HostID
		rightHost := workers[j].HostID
		if leftHost == rightHost {
			leftLabels := strings.Join(workers[i].HostLabels, ",")
			rightLabels := strings.Join(workers[j].HostLabels, ",")
			if leftLabels == rightLabels {
				return workers[i].AgentID < workers[j].AgentID
			}
			return leftLabels < rightLabels
		}
		if leftHost == "" {
			return false
		}
		if rightHost == "" {
			return true
		}
		if workers[i].HostID == workers[j].HostID {
			return workers[i].AgentID < workers[j].AgentID
		}
		return workers[i].HostID < workers[j].HostID
	})
	return workers
}

func EffectiveMaxConcurrency(taskCount, requested int) int {
	if taskCount <= 0 {
		return 0
	}
	if requested <= 0 {
		requested = DefaultMaxConcurrency
	}
	if requested > 64 {
		requested = 64
	}
	if requested > taskCount {
		requested = taskCount
	}
	return requested
}

func NormalizeAgentID(agentID string) string {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return "picoclaw"
	case "zeroclaw":
		return "zeroclaw"
	default:
		return "zeroclaw"
	}
}

func normalizePlannerTasks(tasks []DecomposeTask) []DecomposeTask {
	out := make([]DecomposeTask, 0, len(tasks))
	seen := map[string]struct{}{}
	for idx, task := range tasks {
		input := strings.TrimSpace(task.Input)
		if input == "" {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = fmt.Sprintf("task-%d", idx+1)
		}
		if _, exists := seen[taskID]; exists {
			taskID = fmt.Sprintf("%s-%d", taskID, idx+1)
		}
		seen[taskID] = struct{}{}
		out = append(out, DecomposeTask{
			ID:      taskID,
			Input:   input,
			AgentID: NormalizeAgentID(task.AgentID),
		})
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeSelectorStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}
