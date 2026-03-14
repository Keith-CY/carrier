package gateway

import (
	"fmt"
	"sort"
	"strings"
)

const (
	orchestratorPolicyDecisionAllow = "allow"
	orchestratorPolicyDecisionAsk   = "ask"
	orchestratorPolicyDecisionDeny  = "deny"
)

type OrchestratorPolicyRule struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Enabled            bool     `json:"enabled"`
	Priority           int      `json:"priority,omitempty"`
	Action             string   `json:"action"`
	Reason             string   `json:"reason,omitempty"`
	Teams              []string `json:"teams,omitempty"`
	Projects           []string `json:"projects,omitempty"`
	Environments       []string `json:"environments,omitempty"`
	TemplateIDs        []string `json:"templateIds,omitempty"`
	RequestedProviders []string `json:"requestedProviders,omitempty"`
	HostIDs            []string `json:"hostIds,omitempty"`
	HostLabels         []string `json:"hostLabels,omitempty"`
	AgentIDs           []string `json:"agentIds,omitempty"`
	AllowedTools       []string `json:"allowedTools,omitempty"`
	MaxTaskTimeoutMs   *int     `json:"maxTaskTimeoutMs,omitempty"`
	MaxRetryBudget     *int     `json:"maxRetryBudget,omitempty"`
	CreatedAt          string   `json:"createdAt,omitempty"`
	UpdatedAt          string   `json:"updatedAt,omitempty"`
}

func normalizeOrchestratorPolicyRule(in OrchestratorPolicyRule) OrchestratorPolicyRule {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.Name = strings.TrimSpace(out.Name)
	out.Action = strings.ToLower(strings.TrimSpace(out.Action))
	out.Reason = strings.TrimSpace(out.Reason)
	out.Teams = normalizeStringSelectorList(out.Teams, true)
	out.Projects = normalizeStringSelectorList(out.Projects, true)
	out.Environments = normalizeStringSelectorList(out.Environments, true)
	out.TemplateIDs = normalizeStringSelectorList(out.TemplateIDs, true)
	out.RequestedProviders = normalizeStringSelectorList(out.RequestedProviders, true)
	out.HostIDs = normalizeStringSelectorList(out.HostIDs, false)
	out.HostLabels = normalizeStringSelectorList(out.HostLabels, true)
	out.AgentIDs = normalizeStringSelectorList(out.AgentIDs, true)
	out.AllowedTools = normalizeOrchestratorToolPolicy(OrchestratorToolPolicy{
		Mode:         "restricted",
		AllowedTools: out.AllowedTools,
	}).AllowedTools
	out.MaxTaskTimeoutMs = normalizeOrchestratorPolicyIntLimit(out.MaxTaskTimeoutMs, 1, maxOrchestratorTaskTimeoutMs)
	out.MaxRetryBudget = normalizeOrchestratorPolicyIntLimit(out.MaxRetryBudget, 0, maxOrchestratorTaskRetryBudget)
	if out.Priority < 0 {
		out.Priority = 0
	}
	if out.Priority > 1000 {
		out.Priority = 1000
	}
	return out
}

func validateOrchestratorPolicyRule(in OrchestratorPolicyRule) error {
	rule := normalizeOrchestratorPolicyRule(in)
	if rule.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch rule.Action {
	case orchestratorPolicyDecisionAllow, orchestratorPolicyDecisionAsk, orchestratorPolicyDecisionDeny:
	default:
		return fmt.Errorf("action must be allow, ask, or deny")
	}
	for _, agentID := range rule.AgentIDs {
		if err := validateAgentIdentifier(agentID); err != nil {
			return err
		}
	}
	if rule.MaxTaskTimeoutMs != nil && *rule.MaxTaskTimeoutMs <= 0 {
		return fmt.Errorf("maxTaskTimeoutMs must be greater than 0")
	}
	if rule.MaxRetryBudget != nil && *rule.MaxRetryBudget < 0 {
		return fmt.Errorf("maxRetryBudget must be 0 or greater")
	}
	return nil
}

func normalizeOrchestratorPolicyIntLimit(value *int, minValue, maxValue int) *int {
	if value == nil {
		return nil
	}
	normalized := *value
	if normalized < minValue {
		normalized = minValue
	}
	if normalized > maxValue {
		normalized = maxValue
	}
	return &normalized
}

func normalizeStringSelectorList(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func applyOrchestratorExecutionPolicy(execution OrchestratorExecution, rules []OrchestratorPolicyRule, hosts []RemoteHost) OrchestratorExecution {
	out := execution
	matches := matchingOrchestratorPolicyRules(execution, rules, hosts)
	approvedBy := strings.TrimSpace(out.Policy.ApprovedBy)
	approvedAt := strings.TrimSpace(out.Policy.ApprovedAt)
	decision := orchestratorPolicyDecisionAllow
	reason := ""
	matchedRuleID := ""
	matchedRuleName := ""
	if len(matches) > 0 {
		rule := matches[0]
		out = applyOrchestratorPolicyConstraints(out, rule)
		decision = rule.Action
		reason = rule.Reason
		matchedRuleID = rule.ID
		matchedRuleName = rule.Name
	}
	out.Policy = OrchestratorExecutionPolicySnapshot{
		Decision:        decision,
		Reason:          reason,
		MatchedRuleID:   matchedRuleID,
		MatchedRuleName: matchedRuleName,
		ApprovedBy:      approvedBy,
		ApprovedAt:      approvedAt,
	}
	out.Policy = buildOrchestratorExecutionPolicySnapshot(out)
	return out
}

func matchingOrchestratorPolicyRules(execution OrchestratorExecution, rules []OrchestratorPolicyRule, hosts []RemoteHost) []OrchestratorPolicyRule {
	matches := make([]OrchestratorPolicyRule, 0, len(rules))
	hostIndex := map[string]RemoteHost{}
	for _, host := range hosts {
		hostIndex[strings.TrimSpace(host.ID)] = host
	}
	for _, rule := range sortOrchestratorPolicyRules(rules) {
		if !rule.Enabled {
			continue
		}
		if !orchestratorPolicyRuleMatchesExecution(rule, execution, hostIndex) {
			continue
		}
		matches = append(matches, rule)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := orchestratorPolicyDecisionSeverity(matches[i].Action)
		right := orchestratorPolicyDecisionSeverity(matches[j].Action)
		if left != right {
			return left > right
		}
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority > matches[j].Priority
		}
		leftTime := parseRFC3339OrNow(matches[i].UpdatedAt)
		rightTime := parseRFC3339OrNow(matches[j].UpdatedAt)
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return strings.ToLower(matches[i].ID) < strings.ToLower(matches[j].ID)
	})
	return matches
}

func applyOrchestratorPolicyConstraints(execution OrchestratorExecution, rule OrchestratorPolicyRule) OrchestratorExecution {
	out := execution
	if rule.MaxTaskTimeoutMs != nil {
		limit := *rule.MaxTaskTimeoutMs
		for i := range out.TaskUnits {
			if out.TaskUnits[i].TimeoutMs > limit {
				out.TaskUnits[i].TimeoutMs = limit
			}
		}
	}
	if rule.MaxRetryBudget != nil {
		limit := *rule.MaxRetryBudget
		for i := range out.TaskUnits {
			if out.TaskUnits[i].RetryBudget > limit {
				out.TaskUnits[i].RetryBudget = limit
			}
		}
	}
	if len(rule.AllowedTools) > 0 {
		toolPolicy := normalizeOrchestratorToolPolicy(out.ToolPolicy)
		toolPolicy.Mode = "restricted"
		if len(toolPolicy.AllowedTools) == 0 {
			toolPolicy.AllowedTools = append([]string(nil), rule.AllowedTools...)
		} else {
			toolPolicy.AllowedTools = intersectOrchestratorAllowedTools(toolPolicy.AllowedTools, rule.AllowedTools)
		}
		out.ToolPolicy = normalizeOrchestratorToolPolicy(toolPolicy)
		for i := range out.TaskUnits {
			out.TaskUnits[i].ToolPolicy = "restricted"
		}
	}
	return out
}

func sortOrchestratorPolicyRules(rules []OrchestratorPolicyRule) []OrchestratorPolicyRule {
	out := make([]OrchestratorPolicyRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, normalizeOrchestratorPolicyRule(rule))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		left := parseRFC3339OrNow(out[i].UpdatedAt)
		right := parseRFC3339OrNow(out[j].UpdatedAt)
		if !left.Equal(right) {
			return left.After(right)
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out
}

func orchestratorPolicyRuleMatchesExecution(rule OrchestratorPolicyRule, execution OrchestratorExecution, hostIndex map[string]RemoteHost) bool {
	team := strings.ToLower(strings.TrimSpace(execution.Team))
	project := strings.ToLower(strings.TrimSpace(execution.Project))
	environment := strings.ToLower(strings.TrimSpace(execution.Environment))
	templateID := strings.ToLower(strings.TrimSpace(execution.TemplateID))
	requestedProvider := strings.ToLower(strings.TrimSpace(execution.RequestedProvider))
	targetHosts := executionPolicyTargetHosts(execution)
	targetAgents := executionPolicyTargetAgents(execution)

	if len(rule.Teams) > 0 {
		if team == "" || !stringSliceContainsFold(rule.Teams, team) {
			return false
		}
	}
	if len(rule.Projects) > 0 {
		if project == "" || !stringSliceContainsFold(rule.Projects, project) {
			return false
		}
	}
	if len(rule.Environments) > 0 {
		if environment == "" || !stringSliceContainsFold(rule.Environments, environment) {
			return false
		}
	}
	if len(rule.TemplateIDs) > 0 {
		if templateID == "" || !stringSliceContainsFold(rule.TemplateIDs, templateID) {
			return false
		}
	}
	if len(rule.RequestedProviders) > 0 {
		if requestedProvider == "" || !stringSliceContainsFold(rule.RequestedProviders, requestedProvider) {
			return false
		}
	}
	if len(rule.HostIDs) > 0 && !stringSliceIntersectsFold(rule.HostIDs, targetHosts) {
		return false
	}
	if len(rule.HostLabels) > 0 && !orchestratorPolicyMatchesAnyHostLabels(rule.HostLabels, targetHosts, hostIndex) {
		return false
	}
	if len(rule.AgentIDs) > 0 && !stringSliceIntersectsFold(rule.AgentIDs, targetAgents) {
		return false
	}
	return true
}

func executionPolicyTargetHosts(execution OrchestratorExecution) []string {
	values := make([]string, 0, len(execution.RequiredWorkers)+len(execution.TaskUnits))
	for _, worker := range execution.RequiredWorkers {
		hostID := strings.TrimSpace(worker.HostID)
		if hostID == "" && len(worker.HostLabels) == 0 {
			hostID = orchestratorLocalHostID
		}
		if hostID != "" {
			values = append(values, hostID)
		}
	}
	for _, task := range execution.TaskUnits {
		hostID := strings.TrimSpace(task.HostID)
		if hostID == "" && len(task.HostLabels) == 0 {
			hostID = orchestratorLocalHostID
		}
		if hostID != "" {
			values = append(values, hostID)
		}
	}
	return normalizeStringSelectorList(values, false)
}

func executionPolicyTargetAgents(execution OrchestratorExecution) []string {
	values := make([]string, 0, len(execution.RequiredWorkers)+len(execution.TaskUnits))
	for _, worker := range execution.RequiredWorkers {
		agentID := strings.ToLower(strings.TrimSpace(worker.AgentID))
		if agentID == "" {
			agentID = "zeroclaw"
		}
		values = append(values, agentID)
	}
	for _, task := range execution.TaskUnits {
		agentID := strings.ToLower(strings.TrimSpace(task.AgentID))
		if agentID == "" {
			agentID = "zeroclaw"
		}
		values = append(values, agentID)
	}
	return normalizeStringSelectorList(values, true)
}

func stringSliceContainsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func stringSliceIntersectsFold(left []string, right []string) bool {
	for _, value := range left {
		if stringSliceContainsFold(right, value) {
			return true
		}
	}
	return false
}

func stringSliceContainsAllFold(values []string, required []string) bool {
	for _, candidate := range required {
		if !stringSliceContainsFold(values, candidate) {
			return false
		}
	}
	return true
}

func orchestratorPolicyMatchesAnyHostLabels(requiredLabels []string, targetHosts []string, hostIndex map[string]RemoteHost) bool {
	for _, hostID := range targetHosts {
		host, ok := hostIndex[strings.TrimSpace(hostID)]
		if !ok {
			continue
		}
		if stringSliceContainsAllFold(host.Labels, requiredLabels) {
			return true
		}
	}
	return false
}

func intersectOrchestratorAllowedTools(requested []string, allowed []string) []string {
	filtered := make([]string, 0, len(requested))
	for _, tool := range requested {
		trimmed := strings.TrimSpace(tool)
		if trimmed == "" {
			continue
		}
		if !stringSliceContainsFold(allowed, trimmed) {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return normalizeOrchestratorToolPolicy(OrchestratorToolPolicy{
		Mode:         "restricted",
		AllowedTools: filtered,
	}).AllowedTools
}

func orchestratorPolicyDecisionSeverity(action string) int {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case orchestratorPolicyDecisionDeny:
		return 3
	case orchestratorPolicyDecisionAsk:
		return 2
	case orchestratorPolicyDecisionAllow:
		return 1
	default:
		return 0
	}
}
