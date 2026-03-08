package baseagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const delegateDecomposeSystemPrompt = "You are Carrier's task planner. " +
	"Break a user goal into concrete executable tasks for picoclaw/zeroclaw workers. " +
	"Return JSON only with concise task steps."

var decomposeRequestLLMCompletion = requestLLMCompletionForProvider

type DecomposeTask struct {
	ID      string `json:"id"`
	Input   string `json:"input"`
	AgentID string `json:"agentId,omitempty"`
}

func DecomposeGoal(ctx context.Context, providerID, goal string) ([]DecomposeTask, error) {
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		return nil, errors.New("goal is required")
	}

	userPrompt := strings.Join([]string{
		"Goal:",
		trimmedGoal,
		"",
		"Output JSON only. Use one of these formats:",
		`1) [{"id":"task-1","input":"...","agentId":"picoclaw|zeroclaw(optional)"}]`,
		`2) {"tasks":[...same task schema...]}`,
		"",
		"Rules:",
		"- 1 to 12 tasks",
		"- each task input must be actionable text",
		"- keep task text concise",
	}, "\n")

	raw, err := decomposeRequestLLMCompletion(ctx, strings.TrimSpace(providerID), delegateDecomposeSystemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}
	tasks, err := parseDecomposeTasks(raw)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func parseDecomposeTasks(raw string) ([]DecomposeTask, error) {
	candidate := strings.TrimSpace(extractJSONCandidate(raw))
	if candidate == "" {
		return nil, errors.New("planner returned no JSON payload")
	}

	var arrayPayload []DecomposeTask
	if err := json.Unmarshal([]byte(candidate), &arrayPayload); err == nil {
		return normalizeDecomposeTasks(arrayPayload)
	}

	var objectPayload struct {
		Tasks     []DecomposeTask `json:"tasks"`
		TaskUnits []DecomposeTask `json:"taskUnits"`
		Subtasks  []DecomposeTask `json:"subtasks"`
	}
	if err := json.Unmarshal([]byte(candidate), &objectPayload); err != nil {
		return nil, fmt.Errorf("parse planner output: %w", err)
	}
	switch {
	case len(objectPayload.Tasks) > 0:
		return normalizeDecomposeTasks(objectPayload.Tasks)
	case len(objectPayload.TaskUnits) > 0:
		return normalizeDecomposeTasks(objectPayload.TaskUnits)
	case len(objectPayload.Subtasks) > 0:
		return normalizeDecomposeTasks(objectPayload.Subtasks)
	}
	return nil, errors.New("planner returned no tasks")
}

func normalizeDecomposeTasks(tasks []DecomposeTask) ([]DecomposeTask, error) {
	if len(tasks) == 0 {
		return nil, errors.New("planner returned no tasks")
	}
	if len(tasks) > 12 {
		tasks = tasks[:12]
	}
	out := make([]DecomposeTask, 0, len(tasks))
	seen := map[string]struct{}{}
	for i, task := range tasks {
		input := strings.TrimSpace(task.Input)
		if input == "" {
			continue
		}
		id := strings.TrimSpace(task.ID)
		if id == "" {
			id = fmt.Sprintf("task-%d", i+1)
		}
		if _, ok := seen[id]; ok {
			id = fmt.Sprintf("%s-%d", id, i+1)
		}
		seen[id] = struct{}{}

		agentID := strings.ToLower(strings.TrimSpace(task.AgentID))
		if agentID != "picoclaw" && agentID != "zeroclaw" {
			agentID = ""
		}
		out = append(out, DecomposeTask{
			ID:      id,
			Input:   input,
			AgentID: agentID,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("planner returned no valid tasks")
	}
	return out, nil
}

func extractJSONCandidate(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}

	// Handle fenced code blocks first. Parse each block independently to avoid
	// concatenating content from multiple fenced blocks.
	searchPos := 0
	for searchPos < len(trimmed) {
		startRel := strings.Index(trimmed[searchPos:], "```")
		if startRel < 0 {
			break
		}
		start := searchPos + startRel
		afterFence := start + 3
		lineEndRel := strings.Index(trimmed[afterFence:], "\n")
		if lineEndRel < 0 {
			break
		}
		contentStart := afterFence + lineEndRel + 1
		endRel := strings.Index(trimmed[contentStart:], "```")
		if endRel < 0 {
			break
		}
		payload := strings.TrimSpace(trimmed[contentStart : contentStart+endRel])
		if payload != "" && json.Valid([]byte(payload)) {
			return payload
		}
		searchPos = contentStart + endRel + 3
	}

	firstObject := strings.Index(trimmed, "{")
	firstArray := strings.Index(trimmed, "[")
	start := -1
	if firstObject >= 0 {
		start = firstObject
	}
	if firstArray >= 0 && (start < 0 || firstArray < start) {
		start = firstArray
	}
	if start < 0 {
		return ""
	}
	candidate := strings.TrimSpace(trimmed[start:])
	if json.Valid([]byte(candidate)) {
		return candidate
	}
	lastObject := strings.LastIndex(candidate, "}")
	lastArray := strings.LastIndex(candidate, "]")
	end := lastObject
	if lastArray > end {
		end = lastArray
	}
	if end >= 0 {
		maybe := strings.TrimSpace(candidate[:end+1])
		if json.Valid([]byte(maybe)) {
			return maybe
		}
	}
	return ""
}
