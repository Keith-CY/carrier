package gateway

import (
	"carrier/baseagent"
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func handleInstall(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	spec := baseagent.ActiveBoundarySpec()
	mode := strings.ToLower(strings.TrimSpace(spec.CommandPolicies.ChatInstall))
	if mode == "" {
		mode = "disabled"
	}

	switch mode {
	case "disabled":
		return installViaGUIOnlyResp(cmd.RequestID)
	case "enabled", "requires_host_binding":
	default:
		log.Printf("[gateway] invalid chat_install policy mode %q, fallback to disabled", mode)
		return installViaGUIOnlyResp(cmd.RequestID)
	}

	if len(cmd.Args) == 0 {
		if mode == "requires_host_binding" || spec.CommandPolicies.RequiresExplicitHostForWorkflows {
			return usageResp(cmd.RequestID, "/install <agent_id> <host_id>")
		}
		return usageResp(cmd.RequestID, "/install <agent_id>")
	}

	agentID := strings.TrimSpace(cmd.Args[0])
	if agentID == "" {
		return usageResp(cmd.RequestID, "/install <agent_id>")
	}
	if err := validateAgentIdentifier(agentID); err != nil {
		return errResp(cmd.RequestID, "E_USAGE", err.Error())
	}

	requiresHostBinding := mode == "requires_host_binding" || spec.CommandPolicies.RequiresExplicitHostForWorkflows
	if !requiresHostBinding {
		if daemon == nil {
			return errResp(cmd.RequestID, "E_COMMAND_FAILED", "daemon client is unavailable")
		}
		if err := daemon.InstallAgent(ctx, agentID, actor, cmd.RequestID); err != nil {
			return daemonErrResp(cmd.RequestID, err)
		}
		return GatewayResponse{
			RequestID: cmd.RequestID,
			Result:    "ok",
			Message:   fmt.Sprintf("install completed for %s", agentID),
		}
	}

	hostID := resolveInstallHostID(cmd.Args[1:])
	if hostID == "" {
		return errResp(cmd.RequestID, "E_HOST_BINDING_REQUIRED", "install requires host binding; use /install <agent_id> <host_id>")
	}
	host, ok, err := getRemoteHost(hostID)
	if err != nil {
		return errResp(cmd.RequestID, "E_COMMAND_FAILED", fmt.Sprintf("failed to resolve host binding: %v", err))
	}
	if !ok {
		return errResp(cmd.RequestID, "E_REMOTE_HOST_NOT_FOUND", fmt.Sprintf("remote host %s not found", hostID))
	}
	installCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	workflowResult, installErr := runRemoteInstallWorkflow(installCtx, host, hostID, agentID)
	if installErr != nil {
		return errResp(cmd.RequestID, "E_REMOTE_INSTALL_FAILED", installErr.Error())
	}

	message := fmt.Sprintf("remote install completed for %s on host %s", agentID, hostID)
	if workflowResult.Install != nil && workflowResult.Install.GatewayMode == RemoteRuntimeModeManagedGateway {
		message += " (managed gateway mode)"
	}
	if workflowResult.Attempts > 1 {
		message += fmt.Sprintf("; attempts=%d", workflowResult.Attempts)
	}
	if workflowResult.Repaired {
		message += "; repair_applied=true"
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   message,
	}
}

type remoteInstallWorkflowResult struct {
	Install  *remoteInstallResult
	Repair   *remoteRepairResult
	Attempts int
	Repaired bool
}

func runRemoteInstallWorkflow(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteInstallWorkflowResult, error) {
	out := &remoteInstallWorkflowResult{Attempts: 0}

	preflight, preflightErr := checkRemoteHostAndMaybeRepair(ctx, host)
	if preflightErr != nil {
		return out, fmt.Errorf("remote preflight failed for host %s: %w", hostID, preflightErr)
	}
	if !preflight.SSHOK {
		return out, fmt.Errorf("remote preflight failed for host %s: ssh check did not pass", hostID)
	}

	firstInstall, firstErr := remoteInstallAgent(ctx, host, hostID, agentID, false)
	out.Attempts = 1
	if firstErr == nil {
		out.Install = firstInstall
		return out, nil
	}

	if normalizeRemoteInstallAgentID(agentID) != "openclaw" {
		return out, fmt.Errorf("install failed on host %s: %w", hostID, firstErr)
	}

	repair, repairErr := remoteRepairOpenClaw(ctx, host, hostID, agentID)
	out.Repair = repair
	if repair != nil && repair.Repaired {
		out.Repaired = true
	}
	if repairErr != nil {
		return out, fmt.Errorf("install failed on host %s (%v); repair step failed: %w", hostID, firstErr, repairErr)
	}

	secondInstall, secondErr := remoteInstallAgent(ctx, host, hostID, agentID, false)
	out.Attempts = 2
	if secondErr != nil {
		return out, fmt.Errorf("install failed after repair retry on host %s: %w", hostID, secondErr)
	}
	out.Install = secondInstall
	return out, nil
}

func resolveInstallHostID(args []string) string {
	for _, raw := range args {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(candidate), "host=") {
			candidate = strings.TrimSpace(candidate[len("host="):])
		}
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func handleUninstall(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/uninstall <agent_id>")
	}
	agentID := cmd.Args[0]
	if err := daemon.UninstallAgent(ctx, agentID, actor, cmd.RequestID); err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   fmt.Sprintf("uninstall completed for %s", agentID),
	}
}

func handleStart(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/start <agent_id>")
	}
	agentID := cmd.Args[0]
	if err := daemon.StartAgent(ctx, agentID, actor, cmd.RequestID); err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   fmt.Sprintf("start completed for %s", agentID),
	}
}

func handleStop(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/stop <agent_id>")
	}
	agentID := cmd.Args[0]
	if err := daemon.StopAgent(ctx, agentID, actor, cmd.RequestID); err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   fmt.Sprintf("stop completed for %s", agentID),
	}
}

func handleStatus(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	var agentID string
	if len(cmd.Args) > 0 {
		agentID = cmd.Args[0]
	}
	statuses, err := daemon.GetStatus(ctx, agentID, actor, cmd.RequestID)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	if len(statuses) == 0 {
		return GatewayResponse{
			RequestID: cmd.RequestID,
			Result:    "ok",
			Message:   "no agent status available",
		}
	}
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		uptime := "n/a"
		if s.Runtime == "running" && s.StartedAt != nil {
			if t, err := time.Parse(time.RFC3339Nano, *s.StartedAt); err == nil {
				uptime = formatUptime(time.Since(t))
			}
		}
		ports := "none"
		if len(s.Ports) > 0 {
			portStrs := make([]string, len(s.Ports))
			for i, p := range s.Ports {
				portStrs[i] = strconv.Itoa(p)
			}
			ports = strings.Join(portStrs, ",")
		}
		parts = append(parts, fmt.Sprintf("%s: health=%s runtime=%s version=%s ports=%s uptime=%s restart_count=%d",
			s.ID, s.Health, s.Runtime, s.Version, ports, uptime, s.RestartCount))
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   "status " + strings.Join(parts, "; "),
	}
}

var onlyDigits = regexp.MustCompile(`^\d+$`)

func handleLogs(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, downloads *DownloadStore, actor string) GatewayResponse {
	var agentID string
	var tailStr string

	if len(cmd.Args) == 0 {
	} else if onlyDigits.MatchString(cmd.Args[0]) {
		tailStr = cmd.Args[0]
	} else {
		agentID = cmd.Args[0]
		if len(cmd.Args) > 1 {
			tailStr = cmd.Args[1]
		}
	}
	tail := parsePositiveInt(tailStr, 200)

	var logs *LogsResult
	var err error
	if agentID != "" {
		logs, err = daemon.GetLogs(ctx, agentID, tail, actor, cmd.RequestID)
	} else {
		logs, err = daemon.GetMergedLogs(ctx, tail, actor, cmd.RequestID)
	}
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}

	label := agentID
	if label == "" {
		label = "all agents"
	}
	message := fmt.Sprintf("returned %d log lines for %s", len(logs.Lines), label)
	if len(logs.Lines) == 0 {
		message = fmt.Sprintf("no logs for %s", label)
	}

	resp := GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   message,
	}
	if downloads != nil && (logs.Truncated || len(logs.Lines) > 50) {
		fileLabel := agentID
		if fileLabel == "" {
			fileLabel = "merged"
		}
		logPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-logs-%s.txt", fileLabel, cmd.RequestID))
		content := strings.Join(logs.Lines, "\n")
		if err := os.WriteFile(logPath, []byte(content), 0o600); err == nil {
			tok := downloads.Issue(logPath, 5*time.Minute, true)
			resp.DownloadURL = downloads.ToDownloadURL(tok)
		}
	}
	return resp
}

func handleUpgrade(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/upgrade <agent_id>")
	}
	agentID := cmd.Args[0]
	result, err := daemon.UpgradeAgent(ctx, agentID, actor, cmd.RequestID)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	message := fmt.Sprintf("upgrade completed for %s: %s -> %s", result.AgentID, result.FromVersion, result.ToVersion)
	if result.BackupPath != "" {
		message += fmt.Sprintf(". backup at %s", result.BackupPath)
	}
	if result.RollbackHint != "" {
		message += fmt.Sprintf(". rollback: %s", result.RollbackHint)
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   message,
	}
}

func handleDiagnose(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, downloads *DownloadStore, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/diagnose <agent_id>")
	}
	agentID := cmd.Args[0]
	result, err := daemon.DiagnoseAgent(ctx, agentID, actor, cmd.RequestID)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	resp := GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   fmt.Sprintf("diagnose artifact prepared for %s", agentID),
	}
	if downloads != nil {
		tok := downloads.Issue(result.ArtifactRef, 5*time.Minute, true)
		resp.DownloadURL = downloads.ToDownloadURL(tok)
	}
	return resp
}

func handleDiagnoseConsent(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, downloads *DownloadStore, actor string) GatewayResponse {
	if len(cmd.Args) < 1 {
		return usageResp(cmd.RequestID, "/diagnose-consent <agent_id> <yes|no>")
	}
	agentID := cmd.Args[0]
	consentRaw := ""
	if len(cmd.Args) > 1 {
		consentRaw = cmd.Args[1]
	}
	consent, ok := parseConsent(consentRaw)
	if !ok {
		return errResp(cmd.RequestID, "E_CONSENT_FLAG_INVALID", "expected yes or no")
	}
	handoff, err := daemon.CreateHandoff(ctx, agentID, consent, actor, cmd.RequestID)
	if err != nil {
		if IsRemoteDiagNotNeeded(err) {
			return errResp(cmd.RequestID, "E_REMOTE_DIAG_NOT_NEEDED", err.(*DaemonClientError).Message)
		}
		return daemonErrResp(cmd.RequestID, err)
	}
	resp := GatewayResponse{
		RequestID:     cmd.RequestID,
		Result:        "ok",
		Message:       fmt.Sprintf("remote diagnosis consent recorded for %s", agentID),
		HandoffID:     handoff.ID,
		HandoffStatus: string(handoff.Status),
	}
	if downloads != nil && handoff.ArtifactRef != "" {
		tok := downloads.Issue(handoff.ArtifactRef, 5*time.Minute, true)
		resp.DownloadURL = downloads.ToDownloadURL(tok)
	}
	return resp
}

func parsePositiveInt(s string, fallback int) int {
	if s == "" || !onlyDigits.MatchString(s) {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > math.MaxInt32 {
		return fallback
	}
	return n
}

func parseConsent(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true":
		return true, true
	case "no", "n", "false":
		return false, true
	}
	return false, false
}

func formatUptime(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	total := int(d.Seconds())
	days := total / 86400
	hours := (total % 86400) / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 || secs > 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, "")
}
