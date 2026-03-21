package gateway

import (
	"carrier/baseagent"
	"context"
	"fmt"
	"log"
	"strings"
)

// CommandName is a valid gateway command.
type CommandName string

const (
	CmdPair            CommandName = "/pair"
	CmdChat            CommandName = "/chat"
	CmdDelegate        CommandName = "/delegate"
	CmdAgents          CommandName = "/agents"
	CmdTools           CommandName = "/tools"
	CmdProviders       CommandName = "/providers"
	CmdSessions        CommandName = "/sessions"
	CmdBoundaries      CommandName = "/boundaries"
	CmdAdd             CommandName = "/add"
	CmdInstall         CommandName = "/install"
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
	CmdDelegate:        {},
	CmdAgents:          {},
	CmdTools:           {},
	CmdProviders:       {},
	CmdSessions:        {},
	CmdBoundaries:      {},
	CmdAdd:             {},
	CmdInstall:         {},
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
	RequestID     string                         `json:"requestId"`
	Result        string                         `json:"result"` // "ok" or "error"
	Message       string                         `json:"message"`
	RichContent   *baseagent.RichOutboundMessage `json:"richContent,omitempty"`
	ErrorCode     string                         `json:"errorCode,omitempty"`
	SessionToken  string                         `json:"sessionToken,omitempty"`
	DownloadURL   string                         `json:"downloadUrl,omitempty"`
	HandoffID     string                         `json:"handoffId,omitempty"`
	HandoffStatus string                         `json:"handoffStatus,omitempty"`
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

	if !supportsGatewayCommandsForChannel(provider) {
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

	// All other commands require a valid paired session.
	if authErr := sessions.ValidateSession(cmd.Provider, cmd.ChatID, ""); authErr != nil {
		return errResp(cmd.RequestID, authErr.code, authErr.msg)
	}
	if cmd.SessionToken == "" {
		return errResp(cmd.RequestID, "E_SESSION_TOKEN_MISSING", "session token is required for authenticated commands")
	}
	if authErr := sessions.ValidateSession(cmd.Provider, cmd.ChatID, cmd.SessionToken); authErr != nil {
		return errResp(cmd.RequestID, authErr.code, authErr.msg)
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
	case CmdInstall:
		return handleInstall(ctx, cmd, daemon, actor)
	case CmdOnboard:
		return onboardViaGUIOnlyResp(cmd.RequestID)
	case CmdChat:
		return handleChat(ctx, cmd, daemon, actor)
	case CmdDelegate:
		return handleDelegate(ctx, cmd, daemon, actor)
	case CmdAgents:
		return handleAgents(ctx, cmd, daemon, actor)
	case CmdTools:
		return handleBaseAgentMeta(ctx, cmd, daemon, actor, "/tools")
	case CmdProviders:
		return handleBaseAgentMeta(ctx, cmd, daemon, actor, "/providers")
	case CmdSessions:
		return handleBaseAgentMeta(ctx, cmd, daemon, actor, "/sessions")
	case CmdBoundaries:
		return handleBaseAgentMeta(ctx, cmd, daemon, actor, "/boundaries")
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
	return handleBaseAgentMessage(ctx, cmd, daemon, actor, message)
}

func handleBaseAgentMeta(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor, metaCommand string) GatewayResponse {
	message := strings.TrimSpace(metaCommand)
	if message == "" {
		return errResp(cmd.RequestID, "E_USAGE", "metadata command is empty")
	}
	return handleBaseAgentMessage(ctx, cmd, daemon, actor, message)
}

func handleBaseAgentMessage(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, actor, message string) GatewayResponse {
	chatResult, err := daemon.ChatBaseAgent(ctx, cmd.Provider, cmd.ChatID, cmd.RequestID, message, nil, actor)
	if err != nil {
		return daemonErrResp(cmd.RequestID, err)
	}
	respMessage := strings.TrimSpace(chatResult.Message)
	if respMessage == "" && chatResult.RichContent != nil {
		respMessage = strings.TrimSpace(chatResult.RichContent.PlainTextFallback())
	}
	if respMessage == "" {
		respMessage = "base agent completed with no output"
	}
	return GatewayResponse{
		RequestID:   cmd.RequestID,
		Result:      "ok",
		Message:     respMessage,
		RichContent: chatResult.RichContent,
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
		lines = append(lines, "Tip: `/install` supports remote install with host binding (`/install <agent_id> <host_id>`). Use Carrier CLI/TUI/WebUI for local install and onboarding.")
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
	return errResp(requestID, "E_ADD_GUI_ONLY", "Add/install is disabled in chat to protect credentials. Use Carrier CLI/TUI (`carrier install <agent_id>`, `carrier onboard`) or Carrier WebUI. Chat is for management commands only.")
}

func onboardViaGUIOnlyResp(requestID string) GatewayResponse {
	return errResp(requestID, "E_ONBOARD_GUI_ONLY", "Onboarding is disabled in chat to protect credentials. Use Carrier CLI/TUI (`carrier onboard`) or Carrier WebUI for credential setup.")
}

func installViaGUIOnlyResp(requestID string) GatewayResponse {
	return errResp(requestID, "E_INSTALL_GUI_ONLY", "Install is disabled in chat to protect credentials. Use Carrier CLI/TUI (`carrier install <agent_id>`) or Carrier WebUI.")
}

func daemonErrResp(requestID string, err error) GatewayResponse {
	if de, ok := err.(*DaemonClientError); ok {
		_, code, message := mapDaemonErrorToExternal(de.Code)
		detail := strings.TrimSpace(RedactErrorMessage(de.Message))
		log.Printf("[gateway] daemon command error code=%s detail=%s", code, detail)
		if message == "daemon command failed" && detail != "" {
			message = fmt.Sprintf("%s: %s", message, detail)
		}
		return errResp(requestID, code, message)
	}
	log.Printf("[gateway] daemon command error detail=%s", RedactErrorMessage(err.Error()))
	return errResp(requestID, "E_COMMAND_FAILED", "daemon command failed")
}
