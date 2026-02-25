package gateway

import (
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

// CommandName is a valid gateway command.
type CommandName string

const (
	CmdPair            CommandName = "/pair"
	CmdChat            CommandName = "/chat"
	CmdAgents          CommandName = "/agents"
	CmdAdd             CommandName = "/add"
	CmdUninstall       CommandName = "/uninstall"
	CmdStart           CommandName = "/start"
	CmdStop            CommandName = "/stop"
	CmdStatus          CommandName = "/status"
	CmdLogs            CommandName = "/logs"
	CmdUpgrade         CommandName = "/upgrade"
	CmdDiagnose        CommandName = "/diagnose"
	CmdDiagnoseConsent CommandName = "/diagnose-consent"
	CmdOnboard         CommandName = "/onboard"
)

var validCommands = map[CommandName]struct{}{
	CmdPair:            {},
	CmdChat:            {},
	CmdAgents:          {},
	CmdAdd:             {},
	CmdUninstall:       {},
	CmdStart:           {},
	CmdStop:            {},
	CmdStatus:          {},
	CmdLogs:            {},
	CmdUpgrade:         {},
	CmdDiagnose:        {},
	CmdDiagnoseConsent: {},
	CmdOnboard:         {},
}

var validProviders = map[string]struct{}{
	"telegram": {},
	"discord":  {},
	"feishu":   {},
}

// GatewayCommand is a parsed command from the gateway input.
type GatewayCommand struct {
	Provider     string
	ChatID       string
	RequestID    string
	SessionToken string
	Name         CommandName
	Args         []string
}

// GatewayResponse is the result of a command.
type GatewayResponse struct {
	RequestID     string `json:"requestId"`
	Result        string `json:"result"` // "ok" or "error"
	Message       string `json:"message"`
	ErrorCode     string `json:"errorCode,omitempty"`
	SessionToken  string `json:"sessionToken,omitempty"`
	DownloadURL   string `json:"downloadUrl,omitempty"`
	HandoffID     string `json:"handoffId,omitempty"`
	HandoffStatus string `json:"handoffStatus,omitempty"`
}

// ParseError is returned when command input cannot be parsed.
type ParseError struct {
	RequestID string
	Err       string
}

func (e *ParseError) Error() string { return e.Err }

// ParseInput parses the space-separated command input string.
// Format: <provider> <chatId> <requestId> [session-*] <command> [...args]
func ParseInput(input string) (*GatewayCommand, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) < 4 {
		return nil, &ParseError{RequestID: "unknown", Err: "usage: <provider> <chat_id> <request_id> [session_token] <command> [...args]"}
	}

	provider := fields[0]
	chatID := fields[1]
	requestID := fields[2]
	fourth := fields[3]

	if _, ok := validProviders[provider]; !ok {
		return nil, &ParseError{RequestID: requestID, Err: fmt.Sprintf("unknown provider: %s", provider)}
	}

	var sessionToken string
	var cmdName string
	var args []string

	if strings.HasPrefix(fourth, "session-") {
		sessionToken = fourth
		if len(fields) < 5 {
			return nil, &ParseError{RequestID: requestID, Err: "usage: <provider> <chat_id> <request_id> <session_token> <command> [...args]"}
		}
		cmdName = fields[4]
		args = fields[5:]
	} else {
		cmdName = fourth
		args = fields[4:]
	}

	if _, ok := validCommands[CommandName(cmdName)]; !ok {
		return nil, &ParseError{RequestID: requestID, Err: fmt.Sprintf("unknown command: %s (requestId=%s)", cmdName, requestID)}
	}

	return &GatewayCommand{
		Provider:     provider,
		ChatID:       chatID,
		RequestID:    requestID,
		SessionToken: sessionToken,
		Name:         CommandName(cmdName),
		Args:         args,
	}, nil
}

// HandleCommand executes a parsed command. It is the main dispatcher.
func HandleCommand(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore) GatewayResponse {
	// /pair does not require a session
	if cmd.Name == CmdPair {
		return handlePair(ctx, cmd, daemon, sessions)
	}

	// All other commands require a session
	session := sessions.GetSession(cmd.Provider, cmd.ChatID)
	if session == nil {
		return errResp(cmd.RequestID, "E_SESSION_REQUIRED", "chat is not paired; run /pair <code> first")
	}

	if cmd.SessionToken == "" {
		return errResp(cmd.RequestID, "E_SESSION_TOKEN_MISSING", "session token is required for authenticated commands")
	}
	if cmd.SessionToken != session.SessionToken {
		return errResp(cmd.RequestID, "E_SESSION_TOKEN_INVALID", "session token is invalid")
	}

	sessions.Touch(cmd.Provider, cmd.ChatID)

	if rl != nil {
		key := fmt.Sprintf("%s:%s", cmd.Provider, cmd.ChatID)
		result := rl.Check(key)
		if !result.Allowed {
			return errResp(cmd.RequestID, result.ErrorCode, result.Message)
		}
	}

	actor := fmt.Sprintf("%s:%s", cmd.Provider, cmd.ChatID)

	switch cmd.Name {
	case CmdOnboard:
		return onboardViaGUIOnlyResp(cmd.RequestID)
	case CmdChat:
		return handleChat(ctx, cmd, daemon, actor)
	case CmdAgents:
		return handleAgents(ctx, cmd, daemon, actor)
	case CmdAdd:
		return handleAdd(ctx, cmd, daemon, actor, onboard)
	case CmdUninstall:
		return handleUninstall(ctx, cmd, daemon, actor)
	case CmdStart:
		return handleStart(ctx, cmd, daemon, actor)
	case CmdStop:
		return handleStop(ctx, cmd, daemon, actor)
	case CmdStatus:
		return handleStatus(ctx, cmd, daemon, actor)
	case CmdLogs:
		return handleLogs(ctx, cmd, daemon, downloads, actor)
	case CmdUpgrade:
		return handleUpgrade(ctx, cmd, daemon, actor)
	case CmdDiagnose:
		return handleDiagnose(ctx, cmd, daemon, downloads, actor)
	case CmdDiagnoseConsent:
		return handleDiagnoseConsent(ctx, cmd, daemon, downloads, actor)
	default:
		return errResp(cmd.RequestID, "E_COMMAND_UNSUPPORTED", fmt.Sprintf("unsupported command: %s", cmd.Name))
	}
}

// SafeHandleCommand parses input and dispatches, returning an error GatewayResponse on any failure.
func SafeHandleCommand(ctx context.Context, input string, daemon *DaemonClient, sessions *SessionStore, downloads *DownloadStore, rl *GatewayRateLimiter, onboard *OnboardStore) GatewayResponse {
	cmd, err := ParseInput(input)
	if err != nil {
		if pe, ok := err.(*ParseError); ok {
			return errResp(pe.RequestID, "E_PARSE", pe.Err)
		}
		return errResp("unknown", "E_PARSE", err.Error())
	}
	resp := HandleCommand(ctx, cmd, daemon, sessions, downloads, rl, onboard)
	resp.Message = RedactErrorMessage(resp.Message)
	return resp
}

// InjectSessionToken injects a session token into the input string if the 4th field is a command.
func InjectSessionToken(input, sessionToken string) string {
	if sessionToken == "" {
		return input
	}
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) < 4 {
		return input
	}
	fourth := fields[3]
	if !strings.HasPrefix(fourth, "/") {
		// Already has a session token in 4th position
		return input
	}
	// Insert session token before the command
	result := []string{fields[0], fields[1], fields[2], sessionToken}
	result = append(result, fields[3:]...)
	return strings.Join(result, " ")
}

func handlePair(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, sessions *SessionStore) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/pair <code>")
	}
	code := cmd.Args[0]
	actor := fmt.Sprintf("%s:%s", cmd.Provider, cmd.ChatID)

	if err := daemon.VerifyPairCode(ctx, code, actor, cmd.RequestID); err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}

	session := sessions.CreateSession(cmd.Provider, cmd.ChatID)
	return GatewayResponse{
		RequestID:    cmd.RequestID,
		Result:       "ok",
		Message:      fmt.Sprintf("paired %s:%s", cmd.Provider, cmd.ChatID),
		SessionToken: session.SessionToken,
	}
}

func handleChat(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	if len(cmd.Args) == 0 {
		return usageResp(cmd.RequestID, "/chat <message>")
	}
	message := strings.TrimSpace(strings.Join(cmd.Args, " "))
	if message == "" {
		return usageResp(cmd.RequestID, "/chat <message>")
	}
	chatResult, err := daemon.ChatBaseAgent(ctx, cmd.Provider, cmd.ChatID, cmd.RequestID, message, actor)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	respMessage := strings.TrimSpace(chatResult.Message)
	if respMessage == "" {
		respMessage = "base agent completed with no output"
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   respMessage,
	}
}

func handleAgents(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string) GatewayResponse {
	agents, err := daemon.ListAgents(ctx, actor, cmd.RequestID)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	if len(agents) == 0 {
		return GatewayResponse{
			RequestID: cmd.RequestID,
			Result:    "ok",
			Message:   "listed 0 agents (0 installed)",
		}
	}
	installed := 0
	for _, a := range agents {
		if a.InstallState == "installed" {
			installed++
		}
	}
	lines := []string{fmt.Sprintf("listed %d agents (%d installed)", len(agents), installed)}
	for _, a := range agents {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = a.ID
		}
		installState := strings.TrimSpace(a.InstallState)
		if installState == "" {
			installState = "unknown"
		}
		runtime := strings.TrimSpace(a.Runtime)
		if runtime == "" {
			runtime = "unknown"
		}
		health := strings.TrimSpace(a.Health)
		if health == "" {
			health = "unknown"
		}
		emoji := "⚪"
		if installState != "installed" {
			emoji = "🟡"
		} else if runtime == "running" && health == "healthy" {
			emoji = "🟢"
		} else if runtime == "running" {
			emoji = "🟠"
		}
		lines = append(lines, fmt.Sprintf("%s %s (%s): %s, %s, health=%s", emoji, name, a.ID, installState, runtime, health))
	}
	if installed < len(agents) {
		lines = append(lines, "Tip: install/onboard in Carrier GUI. Chat supports management commands only.")
	}
	return GatewayResponse{
		RequestID: cmd.RequestID,
		Result:    "ok",
		Message:   strings.Join(lines, "\n"),
	}
}

func handleAdd(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor string, onboard *OnboardStore) GatewayResponse {
	return addViaGUIOnlyResp(cmd.RequestID)
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
		// no args → merged logs, default tail
	} else if onlyDigits.MatchString(cmd.Args[0]) {
		// first arg is a number → merged logs with specified tail
		tailStr = cmd.Args[0]
	} else {
		// first arg is agent ID
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
	var message string
	if len(logs.Lines) == 0 {
		message = fmt.Sprintf("no logs for %s", label)
	} else {
		message = fmt.Sprintf("returned %d log lines for %s", len(logs.Lines), label)
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
	var consentRaw string
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

// helpers

func errResp(requestID, code, message string) GatewayResponse {
	return GatewayResponse{
		RequestID: requestID,
		Result:    "error",
		ErrorCode: code,
		Message:   RedactErrorMessage(message),
	}
}

func usageResp(requestID, usage string) GatewayResponse {
	return errResp(requestID, "E_USAGE", "usage: "+usage)
}

func addViaGUIOnlyResp(requestID string) GatewayResponse {
	return errResp(requestID, "E_ADD_GUI_ONLY", "Add/install is disabled in chat to protect credentials. Open Carrier GUI to add/install and onboard agents. Chat is for management commands only.")
}

func onboardViaGUIOnlyResp(requestID string) GatewayResponse {
	return errResp(requestID, "E_ONBOARD_GUI_ONLY", "Onboarding is disabled in chat to protect credentials. Open Carrier GUI to complete onboarding and credential setup.")
}

func daemonErrResp(requestID string, err error) GatewayResponse {
	if de, ok := err.(*DaemonClientError); ok {
		_, code, message := mapDaemonErrorToExternal(de.Code)
		log.Printf("[gateway] daemon command error code=%s detail=%s", code, RedactErrorMessage(de.Message))
		return errResp(requestID, code, message)
	}
	log.Printf("[gateway] daemon command error detail=%s", RedactErrorMessage(err.Error()))
	return errResp(requestID, "E_COMMAND_FAILED", "daemon command failed")
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
