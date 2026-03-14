package gateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxGatewayAuditLogBytes = int64(5 * 1024 * 1024) // 5MB

type gatewayAuditEvent struct {
	Timestamp string                 `json:"timestamp"`
	RequestID string                 `json:"requestId,omitempty"`
	Action    string                 `json:"action"`
	Target    string                 `json:"target,omitempty"`
	Result    string                 `json:"result"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type gatewayAuditExecutionFilter struct {
	ExecutionID string
	Team        string
	Project     string
	TemplateID  string
	Trigger     string
}

var gatewayAuditMu sync.Mutex

func gatewayAuditLogPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_AUDIT_LOG")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for gateway audit log: %w", err)
	}
	return filepath.Join(home, ".carrier", "gateway-audit.jsonl"), nil
}

func emitRemoteAuditEvent(requestID, action, target, result string, details map[string]interface{}) {
	path, err := gatewayAuditLogPath()
	if err != nil {
		return
	}
	event := gatewayAuditEvent{
		Timestamp: nowTimestamp(),
		RequestID: strings.TrimSpace(requestID),
		Action:    strings.TrimSpace(action),
		Target:    strings.TrimSpace(target),
		Result:    strings.TrimSpace(result),
		Details:   sanitizeAuditDetails(details),
	}
	appendGatewayAuditEvent(path, event)
}

func appendGatewayAuditEvent(path string, event gatewayAuditEvent) {
	gatewayAuditMu.Lock()
	defer gatewayAuditMu.Unlock()

	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if stat, err := os.Stat(path); err == nil && stat.Size() > maxGatewayAuditLogBytes {
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()

	row, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = file.Write(append(row, '\n'))
}

func sanitizeAuditDetails(details map[string]interface{}) map[string]interface{} {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(details))
	for key, value := range details {
		switch typed := value.(type) {
		case string:
			out[key] = RedactErrorMessage(strings.TrimSpace(typed))
		default:
			out[key] = value
		}
	}
	return out
}

func listGatewayAuditEventsForExecution(executionID string) ([]gatewayAuditEvent, error) {
	return listGatewayAuditEventsForFilter(gatewayAuditExecutionFilter{ExecutionID: executionID})
}

func listGatewayAuditEventsForFilter(filter gatewayAuditExecutionFilter) ([]gatewayAuditEvent, error) {
	normalized := normalizeGatewayAuditExecutionFilter(filter)
	if gatewayAuditExecutionFilterIsEmpty(normalized) {
		return nil, nil
	}
	path, err := gatewayAuditLogPath()
	if err != nil {
		return nil, err
	}
	events, err := loadGatewayAuditEvents(path)
	if err != nil {
		return nil, err
	}
	executionIDs, err := resolveGatewayAuditExecutionFilterIDs(normalized)
	if err != nil {
		return nil, err
	}
	out := make([]gatewayAuditEvent, 0, len(events))
	for _, event := range events {
		if gatewayAuditEventMatchesAnyExecution(event, executionIDs) {
			out = append(out, event)
		}
	}
	return out, nil
}

func loadGatewayAuditEvents(path string) ([]gatewayAuditEvent, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}
	file, err := os.Open(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]gatewayAuditEvent, 0)
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			var event gatewayAuditEvent
			if err := json.Unmarshal([]byte(line), &event); err == nil {
				events = append(events, event)
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		return nil, readErr
	}
	return events, nil
}

func gatewayAuditEventMatchesExecution(event gatewayAuditEvent, executionID string) bool {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return false
	}
	return gatewayAuditEventMatchesAnyExecution(event, map[string]struct{}{id: {}})
}

func gatewayAuditEventMatchesAnyExecution(event gatewayAuditEvent, executionIDs map[string]struct{}) bool {
	if len(executionIDs) == 0 {
		return false
	}
	if _, ok := executionIDs[strings.TrimSpace(event.Target)]; ok {
		return true
	}
	for _, key := range []string{"executionId", "sourceExecutionId", "parentExecutionId"} {
		if _, ok := executionIDs[auditDetailString(event.Details, key)]; ok {
			return true
		}
	}
	return false
}

func normalizeGatewayAuditExecutionFilter(in gatewayAuditExecutionFilter) gatewayAuditExecutionFilter {
	return gatewayAuditExecutionFilter{
		ExecutionID: strings.TrimSpace(in.ExecutionID),
		Team:        strings.TrimSpace(in.Team),
		Project:     strings.TrimSpace(in.Project),
		TemplateID:  strings.TrimSpace(in.TemplateID),
		Trigger:     strings.TrimSpace(in.Trigger),
	}
}

func gatewayAuditExecutionFilterIsEmpty(filter gatewayAuditExecutionFilter) bool {
	return filter.ExecutionID == "" &&
		filter.Team == "" &&
		filter.Project == "" &&
		filter.TemplateID == "" &&
		filter.Trigger == ""
}

func resolveGatewayAuditExecutionFilterIDs(filter gatewayAuditExecutionFilter) (map[string]struct{}, error) {
	executions, err := listOrchestratorExecutions()
	if err != nil {
		return nil, err
	}
	out := map[string]struct{}{}
	for _, execution := range executions {
		if !gatewayAuditExecutionMatchesFilter(execution, filter) {
			continue
		}
		id := strings.TrimSpace(execution.ID)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func gatewayAuditExecutionMatchesFilter(execution OrchestratorExecution, filter gatewayAuditExecutionFilter) bool {
	if filter.ExecutionID != "" && !strings.EqualFold(strings.TrimSpace(execution.ID), filter.ExecutionID) {
		return false
	}
	if filter.Team != "" && !strings.EqualFold(strings.TrimSpace(execution.Team), filter.Team) {
		return false
	}
	if filter.Project != "" && !strings.EqualFold(strings.TrimSpace(execution.Project), filter.Project) {
		return false
	}
	if filter.TemplateID != "" && !strings.EqualFold(strings.TrimSpace(execution.TemplateID), filter.TemplateID) {
		return false
	}
	if filter.Trigger != "" {
		triggerFilter := strings.ToLower(strings.TrimSpace(filter.Trigger))
		source := strings.ToLower(strings.TrimSpace(execution.TriggerSource))
		triggerID := strings.ToLower(strings.TrimSpace(execution.TriggerID))
		label, _ := executionTriggerAttributionKey(execution)
		if triggerFilter != source && triggerFilter != triggerID && triggerFilter != strings.ToLower(strings.TrimSpace(label)) {
			return false
		}
	}
	return true
}

func auditDetailString(details map[string]interface{}, key string) string {
	if len(details) == 0 {
		return ""
	}
	value, ok := details[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
