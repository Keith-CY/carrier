package gateway

import (
	"carrier/shared/orchestration"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ExecutionTriggerType string

const (
	ExecutionTriggerTypeWebhook  ExecutionTriggerType = "webhook"
	ExecutionTriggerTypeGitHub   ExecutionTriggerType = "github"
	ExecutionTriggerTypeSchedule ExecutionTriggerType = "schedule"
)

type ExecutionTriggerConfig struct {
	Inputs                  map[string]string `json:"inputs,omitempty"`
	Provider                string            `json:"provider,omitempty"`
	HostIDs                 []string          `json:"hostIds,omitempty"`
	HostLabels              []string          `json:"hostLabels,omitempty"`
	MaxConcurrency          int               `json:"maxConcurrency,omitempty"`
	PolicyApprove           bool              `json:"policyApprove,omitempty"`
	WebhookSecret           string            `json:"webhookSecret,omitempty"`
	WebhookSecretConfigured bool              `json:"webhookSecretConfigured,omitempty"`
	GitHubCommand           string            `json:"githubCommand,omitempty"`
	GitHubLabel             string            `json:"githubLabel,omitempty"`
	GitHubRepository        string            `json:"githubRepository,omitempty"`
	Cron                    string            `json:"cron,omitempty"`
	Timezone                string            `json:"timezone,omitempty"`
}

type ExecutionTrigger struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Type            ExecutionTriggerType `json:"type"`
	TemplateID      string               `json:"templateId"`
	Enabled         bool                 `json:"enabled"`
	CreatedBy       string               `json:"createdBy,omitempty"`
	Config          ExecutionTriggerConfig `json:"config,omitempty"`
	LastTriggeredAt string               `json:"lastTriggeredAt,omitempty"`
	LastExecutionID string               `json:"lastExecutionId,omitempty"`
	LastError       string               `json:"lastError,omitempty"`
	TriggeredCount  int64                `json:"triggeredCount,omitempty"`
	NextRunAt       string               `json:"nextRunAt,omitempty"`
	CreatedAt       string               `json:"createdAt,omitempty"`
	UpdatedAt       string               `json:"updatedAt,omitempty"`
}

func normalizeExecutionTriggerType(raw string) ExecutionTriggerType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ExecutionTriggerTypeWebhook):
		return ExecutionTriggerTypeWebhook
	case string(ExecutionTriggerTypeGitHub):
		return ExecutionTriggerTypeGitHub
	case string(ExecutionTriggerTypeSchedule):
		return ExecutionTriggerTypeSchedule
	default:
		return ""
	}
}

func normalizeExecutionTriggerConfig(in ExecutionTriggerConfig) ExecutionTriggerConfig {
	out := in
	out.Provider = strings.TrimSpace(out.Provider)
	out.HostLabels = normalizeStringSelectorList(out.HostLabels, true)
	hostIDs := make([]string, 0, len(out.HostIDs))
	seenHostIDs := map[string]struct{}{}
	for _, hostID := range out.HostIDs {
		trimmed := strings.TrimSpace(hostID)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seenHostIDs[key]; ok {
			continue
		}
		seenHostIDs[key] = struct{}{}
		hostIDs = append(hostIDs, trimmed)
	}
	out.HostIDs = hostIDs
	if out.MaxConcurrency < 0 {
		out.MaxConcurrency = 0
	}
	if out.MaxConcurrency > 64 {
		out.MaxConcurrency = 64
	}
	out.WebhookSecret = strings.TrimSpace(out.WebhookSecret)
	out.WebhookSecretConfigured = out.WebhookSecret != ""
	out.GitHubCommand = strings.TrimSpace(out.GitHubCommand)
	out.GitHubLabel = strings.TrimSpace(out.GitHubLabel)
	out.GitHubRepository = strings.TrimSpace(out.GitHubRepository)
	out.Cron = strings.TrimSpace(out.Cron)
	out.Timezone = strings.ToUpper(strings.TrimSpace(out.Timezone))
	if out.Timezone == "" {
		out.Timezone = "UTC"
	}
	if out.Inputs == nil {
		out.Inputs = map[string]string{}
	} else {
		normalizedInputs := make(map[string]string, len(out.Inputs))
		for key, value := range out.Inputs {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			normalizedInputs[trimmedKey] = strings.TrimSpace(value)
		}
		out.Inputs = normalizedInputs
	}
	return out
}

func normalizeExecutionTriggerForStore(in ExecutionTrigger) (ExecutionTrigger, error) {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	if out.ID == "" {
		out.ID = uuid.NewString()
	}
	out.Name = strings.TrimSpace(out.Name)
	out.Type = normalizeExecutionTriggerType(string(out.Type))
	out.TemplateID = strings.TrimSpace(out.TemplateID)
	out.CreatedBy = strings.TrimSpace(out.CreatedBy)
	out.LastTriggeredAt = strings.TrimSpace(out.LastTriggeredAt)
	out.LastExecutionID = strings.TrimSpace(out.LastExecutionID)
	out.LastError = strings.TrimSpace(out.LastError)
	out.NextRunAt = strings.TrimSpace(out.NextRunAt)
	out.Config = normalizeExecutionTriggerConfig(out.Config)
	if out.Type == "" {
		return ExecutionTrigger{}, fmt.Errorf("trigger type is required")
	}
	if out.TemplateID == "" {
		return ExecutionTrigger{}, fmt.Errorf("templateId is required")
	}
	if _, ok := orchestration.GetExecutionTemplate(out.TemplateID); !ok {
		return ExecutionTrigger{}, fmt.Errorf("template %s not found", out.TemplateID)
	}
	if out.Name == "" {
		out.Name = fmt.Sprintf("%s %s", out.TemplateID, out.Type)
	}
	switch out.Type {
	case ExecutionTriggerTypeWebhook:
		if out.Config.WebhookSecret == "" {
			return ExecutionTrigger{}, fmt.Errorf("webhookSecret is required for webhook triggers")
		}
	case ExecutionTriggerTypeGitHub:
		if out.Config.WebhookSecret == "" {
			return ExecutionTrigger{}, fmt.Errorf("webhookSecret is required for github triggers")
		}
		if out.Config.GitHubCommand == "" && out.Config.GitHubLabel == "" {
			return ExecutionTrigger{}, fmt.Errorf("githubCommand or githubLabel is required for github triggers")
		}
	case ExecutionTriggerTypeSchedule:
		if out.Config.Cron == "" {
			return ExecutionTrigger{}, fmt.Errorf("cron is required for schedule triggers")
		}
		if out.Config.Timezone != "UTC" {
			return ExecutionTrigger{}, fmt.Errorf("schedule trigger timezone must be UTC")
		}
	default:
		return ExecutionTrigger{}, fmt.Errorf("unsupported trigger type %s", out.Type)
	}
	return out, nil
}

func sanitizeExecutionTrigger(in ExecutionTrigger) ExecutionTrigger {
	out := in
	out.Config = normalizeExecutionTriggerConfig(out.Config)
	out.Config.WebhookSecretConfigured = out.Config.WebhookSecret != ""
	out.Config.WebhookSecret = ""
	return out
}
