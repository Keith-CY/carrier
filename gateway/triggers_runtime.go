package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func startExecutionTriggerScheduler(ctx context.Context, cfg *GatewayConfig) {
	if cfg == nil || !effectiveGatewayFeatureFlags(cfg).RemoteControlPlaneEnabled {
		return
	}
	interval := cfg.TriggerSchedulePollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := runScheduledExecutionTriggers(time.Now().UTC(), "scheduler", cfg); err != nil {
					log.Printf("[gateway] trigger scheduler failed: %v", err)
				}
			}
		}
	}()
}

func launchExecutionTriggerFromWebhook(requestID string, cfg *GatewayConfig, trigger ExecutionTrigger, r *http.Request, body []byte) (map[string]interface{}, *gatewayAPIResponseError) {
	payloadMap, err := decodeExecutionTriggerPayload(body)
	if err != nil {
		return nil, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", err.Error())}
	}
	payloadDigest := sha256Hex(body)
	payloadContext := flattenExecutionTriggerPayload(payloadMap)

	metadata := executionLaunchMetadata{
		TriggerSource:       string(trigger.Type),
		TriggerID:           trigger.ID,
		TriggerPayloadDigest: payloadDigest,
		Initiator:           "webhook:" + trigger.ID,
	}
	switch trigger.Type {
	case ExecutionTriggerTypeWebhook:
		if !subtleTokenCompare(strings.TrimSpace(r.Header.Get("X-Carrier-Trigger-Secret")), trigger.Config.WebhookSecret) {
			return nil, &gatewayAPIResponseError{Status: http.StatusUnauthorized, Body: gatewayErrBody("E_TRIGGER_SECRET_INVALID", "invalid trigger secret")}
		}
		metadata.TriggerEvent = "webhook"
	case ExecutionTriggerTypeGitHub:
		if !verifyGitHubWebhookSignature(trigger.Config.WebhookSecret, strings.TrimSpace(r.Header.Get("X-Hub-Signature-256")), body) {
			return nil, &gatewayAPIResponseError{Status: http.StatusUnauthorized, Body: gatewayErrBody("E_TRIGGER_SIGNATURE_INVALID", "invalid github webhook signature")}
		}
		event := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
		matched, reason := shouldLaunchGitHubTrigger(trigger, event, payloadContext)
		if !matched {
			return map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"triggered": false,
				"reason":    reason,
			}, nil
		}
		metadata.TriggerEvent = event
		if sender := strings.TrimSpace(payloadContext["payload.sender.login"]); sender != "" {
			metadata.Initiator = "github:" + sender
		} else {
			metadata.Initiator = "github:webhook"
		}
	default:
		return nil, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", "trigger type does not support webhook launches")}
	}

	inputs := renderExecutionTriggerInputs(trigger.Config.Inputs, payloadContext)
	idempotencyKey := fmt.Sprintf("trigger:%s:%s:%s", trigger.ID, strings.TrimSpace(metadata.TriggerEvent), payloadDigest)
	execution, apiErr := launchExecutionTemplate(requestID, cfg, executionTemplateLaunchOptions{
		TemplateID:     trigger.TemplateID,
		Inputs:         inputs,
		Provider:       trigger.Config.Provider,
		HostIDs:        trigger.Config.HostIDs,
		HostLabels:     trigger.Config.HostLabels,
		MaxConcurrency: trigger.Config.MaxConcurrency,
		PolicyApprove:  trigger.Config.PolicyApprove,
		IdempotencyKey: idempotencyKey,
		Actor:          metadata.Initiator,
		Metadata:       metadata,
	})
	if apiErr != nil {
		markExecutionTriggerLaunch(trigger, "", gatewayAPIResponseErrorMessage(apiErr), "", false)
		return nil, apiErr
	}
	markExecutionTriggerLaunch(trigger, execution.ID, "", "", true)
	emitRemoteAuditEvent(requestID, "orchestrator_trigger_fire", trigger.ID, "success", map[string]interface{}{
		"type":            trigger.Type,
		"templateId":      trigger.TemplateID,
		"executionId":     execution.ID,
		"triggerSource":   execution.TriggerSource,
		"triggerEvent":    execution.TriggerEvent,
		"payloadDigest":   execution.TriggerPayloadDigest,
		"initiator":       execution.Initiator,
	})
	return map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"triggered": true,
		"trigger":   sanitizeExecutionTrigger(trigger),
		"execution": execution,
	}, nil
}

func runScheduledExecutionTriggers(now time.Time, requestID string, cfg *GatewayConfig) error {
	triggers, err := listExecutionTriggers()
	if err != nil {
		return err
	}
	for _, trigger := range triggers {
		if trigger.Type != ExecutionTriggerTypeSchedule || !trigger.Enabled {
			continue
		}
		dueAt, due, err := executionTriggerDueAt(trigger, now.UTC())
		if err != nil {
			markExecutionTriggerLaunch(trigger, "", err.Error(), "", false)
			continue
		}
		if !due {
			continue
		}
		execution, apiErr := launchExecutionTemplate(requestID, cfg, executionTemplateLaunchOptions{
			TemplateID:     trigger.TemplateID,
			Inputs:         renderExecutionTriggerInputs(trigger.Config.Inputs, nil),
			Provider:       trigger.Config.Provider,
			HostIDs:        trigger.Config.HostIDs,
			HostLabels:     trigger.Config.HostLabels,
			MaxConcurrency: trigger.Config.MaxConcurrency,
			PolicyApprove:  trigger.Config.PolicyApprove,
			IdempotencyKey: fmt.Sprintf("trigger:%s:schedule:%s", trigger.ID, dueAt.Format(time.RFC3339)),
			Actor:          "schedule:" + trigger.ID,
			Metadata: executionLaunchMetadata{
				TriggerSource: string(ExecutionTriggerTypeSchedule),
				TriggerID:     trigger.ID,
				TriggerEvent:  "schedule",
				Initiator:     "schedule:" + trigger.ID,
			},
		})
		nextRunAt, nextErr := nextExecutionTriggerRunAt(dueAt, trigger.Config.Cron)
		if apiErr != nil {
			nextRun := ""
			if nextErr == nil {
				nextRun = nextRunAt.Format(time.RFC3339)
			}
			markExecutionTriggerLaunch(trigger, "", gatewayAPIResponseErrorMessage(apiErr), nextRun, false)
			continue
		}
		nextRun := ""
		if nextErr == nil {
			nextRun = nextRunAt.Format(time.RFC3339)
		}
		markExecutionTriggerLaunch(trigger, execution.ID, "", nextRun, true)
		emitRemoteAuditEvent(requestID, "orchestrator_trigger_fire", trigger.ID, "success", map[string]interface{}{
			"type":        trigger.Type,
			"templateId":  trigger.TemplateID,
			"executionId": execution.ID,
			"triggerEvent": "schedule",
			"initiator":   execution.Initiator,
		})
	}
	return nil
}

func executionTriggerDueAt(trigger ExecutionTrigger, now time.Time) (time.Time, bool, error) {
	if strings.TrimSpace(trigger.NextRunAt) == "" {
		nextRunAt, err := nextExecutionTriggerRunAt(now, trigger.Config.Cron)
		if err != nil {
			return time.Time{}, false, err
		}
		markExecutionTriggerLaunch(trigger, "", "", nextRunAt.Format(time.RFC3339), false)
		return nextRunAt, false, nil
	}
	dueAt, err := time.Parse(time.RFC3339, strings.TrimSpace(trigger.NextRunAt))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid nextRunAt for trigger %s", trigger.ID)
	}
	return dueAt.UTC(), !dueAt.After(now.UTC()), nil
}

func markExecutionTriggerLaunch(trigger ExecutionTrigger, executionID, lastError, nextRunAt string, launched bool) {
	updated := trigger
	if launched {
		updated.LastTriggeredAt = nowTimestamp()
		updated.LastExecutionID = strings.TrimSpace(executionID)
		updated.LastError = ""
		updated.TriggeredCount++
	} else if strings.TrimSpace(lastError) != "" {
		updated.LastError = strings.TrimSpace(lastError)
	}
	if strings.TrimSpace(nextRunAt) != "" || updated.Type == ExecutionTriggerTypeSchedule {
		updated.NextRunAt = strings.TrimSpace(nextRunAt)
	}
	_, _ = upsertExecutionTrigger(updated)
}

func renderExecutionTriggerInputs(inputs map[string]string, context map[string]string) map[string]string {
	out := make(map[string]string, len(inputs))
	for key, value := range inputs {
		out[strings.TrimSpace(key)] = renderExecutionTriggerString(value, context)
	}
	return out
}

func renderExecutionTriggerString(text string, context map[string]string) string {
	rendered := strings.TrimSpace(text)
	for key, value := range context {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", strings.TrimSpace(value))
	}
	return strings.Join(strings.Fields(rendered), " ")
}

func decodeExecutionTriggerPayload(body []byte) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return map[string]interface{}{}, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("request body must be valid JSON")
	}
	return payload, nil
}

func flattenExecutionTriggerPayload(payload map[string]interface{}) map[string]string {
	out := map[string]string{}
	for key, value := range payload {
		flattenExecutionTriggerValue("payload."+key, value, out)
	}
	return out
}

func flattenExecutionTriggerValue(prefix string, value interface{}, out map[string]string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			flattenExecutionTriggerValue(prefix+"."+key, nested, out)
		}
	case []interface{}:
		for idx, nested := range typed {
			flattenExecutionTriggerValue(prefix+"."+strconv.Itoa(idx), nested, out)
		}
	case string:
		out[prefix] = typed
	case float64:
		if typed == float64(int64(typed)) {
			out[prefix] = strconv.FormatInt(int64(typed), 10)
		} else {
			out[prefix] = strconv.FormatFloat(typed, 'f', -1, 64)
		}
	case bool:
		out[prefix] = strconv.FormatBool(typed)
	case nil:
	default:
		out[prefix] = strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func shouldLaunchGitHubTrigger(trigger ExecutionTrigger, event string, payload map[string]string) (bool, string) {
	repository := strings.TrimSpace(payload["payload.repository.full_name"])
	if trigger.Config.GitHubRepository != "" && !strings.EqualFold(trigger.Config.GitHubRepository, repository) {
		return false, "repository did not match trigger filter"
	}
	commentMatched := false
	labelMatched := false
	switch strings.TrimSpace(event) {
	case "issue_comment":
		if strings.TrimSpace(payload["payload.action"]) == "created" && trigger.Config.GitHubCommand != "" {
			body := strings.TrimSpace(payload["payload.comment.body"])
			commentMatched = strings.HasPrefix(body, trigger.Config.GitHubCommand)
		}
	case "issues":
		if strings.TrimSpace(payload["payload.action"]) == "labeled" && trigger.Config.GitHubLabel != "" {
			labelMatched = strings.EqualFold(strings.TrimSpace(payload["payload.label.name"]), trigger.Config.GitHubLabel)
		}
	}
	if commentMatched || labelMatched {
		return true, ""
	}
	return false, "github event did not match trigger conditions"
}

func verifyGitHubWebhookSignature(secret, signature string, body []byte) bool {
	trimmedSecret := strings.TrimSpace(secret)
	trimmedSignature := strings.TrimSpace(signature)
	if trimmedSecret == "" || trimmedSignature == "" || !strings.HasPrefix(trimmedSignature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(trimmedSecret))
	_, _ = mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(trimmedSignature))
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func gatewayAPIResponseErrorMessage(apiErr *gatewayAPIResponseError) string {
	if apiErr == nil || apiErr.Body == nil {
		return ""
	}
	if message, ok := apiErr.Body["message"].(string); ok {
		return strings.TrimSpace(message)
	}
	if errMap, ok := apiErr.Body["error"].(map[string]interface{}); ok {
		if message, ok := errMap["message"].(string); ok {
			return strings.TrimSpace(message)
		}
	}
	return ""
}

func nextExecutionTriggerRunAt(after time.Time, cronExpr string) (time.Time, error) {
	schedule, err := parseExecutionTriggerCron(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	candidate := after.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if schedule.matches(candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching run found within one year")
}

type executionTriggerCronSchedule struct {
	minute  cronFieldMatcher
	hour    cronFieldMatcher
	day     cronFieldMatcher
	month   cronFieldMatcher
	weekday cronFieldMatcher
}

type cronFieldMatcher struct {
	any    bool
	step   int
	values map[int]struct{}
}

func parseExecutionTriggerCron(expr string) (executionTriggerCronSchedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return executionTriggerCronSchedule{}, fmt.Errorf("cron must contain 5 fields")
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return executionTriggerCronSchedule{}, fmt.Errorf("invalid cron minute: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return executionTriggerCronSchedule{}, fmt.Errorf("invalid cron hour: %w", err)
	}
	day, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return executionTriggerCronSchedule{}, fmt.Errorf("invalid cron day: %w", err)
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return executionTriggerCronSchedule{}, fmt.Errorf("invalid cron month: %w", err)
	}
	weekday, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return executionTriggerCronSchedule{}, fmt.Errorf("invalid cron weekday: %w", err)
	}
	return executionTriggerCronSchedule{minute: minute, hour: hour, day: day, month: month, weekday: weekday}, nil
}

func (s executionTriggerCronSchedule) matches(at time.Time) bool {
	return s.minute.matches(at.Minute()) &&
		s.hour.matches(at.Hour()) &&
		s.day.matches(at.Day()) &&
		s.month.matches(int(at.Month())) &&
		s.weekday.matches(int(at.Weekday()))
}

func parseCronField(raw string, min, max int) (cronFieldMatcher, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "*" {
		return cronFieldMatcher{any: true}, nil
	}
	if strings.HasPrefix(trimmed, "*/") {
		step, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "*/")))
		if err != nil || step <= 0 {
			return cronFieldMatcher{}, fmt.Errorf("invalid step")
		}
		return cronFieldMatcher{step: step}, nil
	}
	values := map[int]struct{}{}
	for _, part := range strings.Split(trimmed, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return cronFieldMatcher{}, fmt.Errorf("invalid value %q", part)
		}
		if value < min || value > max {
			return cronFieldMatcher{}, fmt.Errorf("value %d out of range", value)
		}
		values[value] = struct{}{}
	}
	if len(values) == 0 {
		return cronFieldMatcher{}, fmt.Errorf("empty field")
	}
	return cronFieldMatcher{values: values}, nil
}

func (m cronFieldMatcher) matches(value int) bool {
	if m.any {
		return true
	}
	if m.step > 0 {
		return value%m.step == 0
	}
	_, ok := m.values[value]
	return ok
}
