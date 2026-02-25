package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"carrier/shared/redact"
)

const installFailureTriagerSystemPrompt = "You are Carrier's install-failure triage engine. " +
	"Return JSON only. " +
	"Never ask for secrets. Never emit destructive commands. " +
	"Only suggest a repairAction when a single low-risk command can help recover install failures. " +
	"Allowed command values: " +
	"`npm cache clean --force`, `pnpm install`, `npm install`, `yarn install`, `pip cache purge`, `pip install -r requirements.txt`, `go mod download`. " +
	"If unsure, set repairAction to null and requiresRemoteDiagnosis=true."

type LLMTriager struct {
	fallback Triager
}

type llmRepairAction struct {
	Command    string `json:"command"`
	TargetPath string `json:"targetPath"`
	RiskLevel  string `json:"riskLevel"`
}

type llmTriageResponse struct {
	Resolved                bool             `json:"resolved"`
	Summary                 string           `json:"summary"`
	SuggestedActions        []string         `json:"suggestedActions"`
	RequiresRemoteDiagnosis bool             `json:"requiresRemoteDiagnosis"`
	RepairAction            *llmRepairAction `json:"repairAction"`
}

func NewLLMTriager(fallback Triager) *LLMTriager {
	if fallback == nil {
		fallback = NoopTriager{}
	}
	return &LLMTriager{fallback: fallback}
}

func (t *LLMTriager) Analyze(ctx context.Context, e Evidence) (TriageResult, error) {
	prompt := buildInstallFailurePrompt(e)
	raw, err := requestLLMCompletion(ctx, installFailureTriagerSystemPrompt, prompt)
	if err != nil {
		return t.fallbackResult(ctx, e, err), nil
	}

	parsed, err := parseLLMTriageResponse(raw)
	if err != nil {
		return t.fallbackResult(ctx, e, err), nil
	}

	result := TriageResult{
		Resolved:                parsed.Resolved,
		Summary:                 strings.TrimSpace(parsed.Summary),
		SuggestedActions:        compactNonEmpty(parsed.SuggestedActions, 8),
		RequiresRemoteDiagnosis: parsed.RequiresRemoteDiagnosis,
	}
	if parsed.RepairAction != nil {
		command := strings.TrimSpace(parsed.RepairAction.Command)
		if command != "" {
			result.RepairAction = &RepairAction{
				Command:    command,
				TargetPath: strings.TrimSpace(parsed.RepairAction.TargetPath),
				RiskLevel:  RiskLevel(strings.ToLower(strings.TrimSpace(parsed.RepairAction.RiskLevel))),
			}
		}
	}
	if result.Summary == "" {
		result.Summary = "LLM triage produced no summary; using fallback guidance."
	}
	return result, nil
}

func (t *LLMTriager) fallbackResult(ctx context.Context, e Evidence, cause error) TriageResult {
	base, _ := t.fallback.Analyze(ctx, e)
	reason := strings.TrimSpace(strings.ReplaceAll(cause.Error(), "\n", " "))
	if len(reason) > 240 {
		reason = reason[:240] + "..."
	}
	if base.Summary == "" {
		base.Summary = "Base Agent fallback triage activated."
	}
	base.Summary = fmt.Sprintf("%s (LLM unavailable: %s)", base.Summary, reason)
	base.RepairAction = nil
	base.RequiresRemoteDiagnosis = true
	if len(base.SuggestedActions) == 0 {
		base.SuggestedActions = []string{"Run /diagnose", "Review latest logs"}
	}
	return base
}

func buildInstallFailurePrompt(e Evidence) string {
	const maxTail = 40
	logTail := e.LogTail
	if len(logTail) > maxTail {
		logTail = logTail[len(logTail)-maxTail:]
	}
	redactedLogs := make([]string, 0, len(logTail))
	for _, line := range logTail {
		redactedLogs = append(redactedLogs, redact.RedactText(line))
	}

	payload := map[string]interface{}{
		"agentId":     strings.TrimSpace(e.AgentID),
		"lastError":   redact.RedactText(strings.TrimSpace(e.LastError)),
		"exitCode":    e.ExitCode,
		"logTail":     redactedLogs,
		"healthProbe": redact.RedactText(strings.TrimSpace(e.HealthProbe)),
		"outputSchema": map[string]interface{}{
			"resolved":                "boolean",
			"summary":                 "string",
			"suggestedActions":        []string{},
			"requiresRemoteDiagnosis": "boolean",
			"repairAction": map[string]string{
				"command":    "string (must be in allowlist) or empty",
				"targetPath": "string (optional)",
				"riskLevel":  "low",
			},
		},
	}
	raw, _ := json.Marshal(payload)
	return "Analyze the install failure evidence and return exactly one JSON object matching outputSchema.\n" + string(raw)
}

func parseLLMTriageResponse(raw string) (*llmTriageResponse, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil, fmt.Errorf("empty model response")
	}

	// Tolerate markdown wrappers and extra narration by extracting the JSON object.
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("triage response did not contain JSON object")
	}
	candidate = candidate[start : end+1]

	var parsed llmTriageResponse
	if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
		return nil, fmt.Errorf("decode triage json: %w", err)
	}
	return &parsed, nil
}

func compactNonEmpty(items []string, limit int) []string {
	if limit <= 0 {
		limit = len(items)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
		if len(out) >= limit {
			break
		}
	}
	return out
}
