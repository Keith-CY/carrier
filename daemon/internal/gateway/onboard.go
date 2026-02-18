package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// OnboardStep is a step in the onboard state machine.
type OnboardStep string

const (
	OnboardIdle         OnboardStep = "idle"
	OnboardAgentSelected OnboardStep = "agent_selected"
	OnboardEnvConfigured OnboardStep = "env_configured"
	OnboardInstalling   OnboardStep = "installing"
	OnboardDone         OnboardStep = "done"
)

// OnboardSession is the state for a single chat's onboard flow.
type OnboardSession struct {
	Step          OnboardStep
	SelectedAgent string
	EnvVars       map[string]string
}

// OnboardStore tracks per-session onboard state.
type OnboardStore struct {
	mu       sync.Mutex
	sessions map[string]*OnboardSession
}

// NewOnboardStore creates a new onboard store.
func NewOnboardStore() *OnboardStore {
	return &OnboardStore{sessions: make(map[string]*OnboardSession)}
}

func (s *OnboardStore) get(key string) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key]
}

func (s *OnboardStore) start(key string) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := &OnboardSession{Step: OnboardIdle, EnvVars: make(map[string]string)}
	s.sessions[key] = sess
	return sess
}

func (s *OnboardStore) update(key string, fn func(*OnboardSession)) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[key]
	if sess == nil {
		sess = &OnboardSession{Step: OnboardIdle, EnvVars: make(map[string]string)}
		s.sessions[key] = sess
	}
	fn(sess)
	return sess
}

func (s *OnboardStore) clear(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

func (s *OnboardStore) hasActive(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[key]
	return sess != nil && sess.Step != OnboardIdle && sess.Step != OnboardDone
}

// handleOnboard handles the /onboard command.
func handleOnboard(ctx context.Context, cmd *GatewayCommand, daemon *DaemonClient, store *OnboardStore) GatewayResponse {
	if store == nil {
		store = NewOnboardStore()
	}
	actor := fmt.Sprintf("%s:%s", cmd.Provider, cmd.ChatID)
	sessionKey := actor

	var firstArg string
	if len(cmd.Args) > 0 {
		firstArg = cmd.Args[0]
	}

	// status always works
	if firstArg == "status" {
		return onboardStatus(ctx, cmd.RequestID, daemon, actor)
	}

	// cancel aborts active session
	if firstArg == "cancel" {
		return onboardCancel(cmd.RequestID, sessionKey, store)
	}

	// no args → start interactive session
	if firstArg == "" {
		return onboardStart(ctx, cmd.RequestID, sessionKey, daemon, store, actor)
	}

	// If active session, route as reply
	if store.hasActive(sessionKey) {
		return onboardReply(ctx, cmd.RequestID, sessionKey, cmd.Args, daemon, store, actor)
	}

	// No active session with arg → agent selection shortcut
	store.start(sessionKey)
	return onboardReply(ctx, cmd.RequestID, sessionKey, cmd.Args, daemon, store, actor)
}

func onboardStart(ctx context.Context, requestID, sessionKey string, daemon *DaemonClient, store *OnboardStore, actor string) GatewayResponse {
	agents, err := daemon.ListAgents(ctx, actor, requestID)
	if err != nil {
		return daemonErrResp(requestID, err)
	}
	store.start(sessionKey)

	lines := []string{"🚀 Welcome to Carrier! Let's set up your agents.", ""}
	lines = append(lines, "Available agents:")
	for i, a := range agents {
		tag := ""
		if a.InstallState == "installed" {
			tag = " [installed]"
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s)%s", i+1, a.Name, a.ID, tag))
	}
	lines = append(lines, "")
	lines = append(lines, "Reply with the agent name to install (e.g. `/onboard openclaw`), or `/onboard cancel` to abort.")
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
}

func onboardReply(ctx context.Context, requestID, sessionKey string, args []string, daemon *DaemonClient, store *OnboardStore, actor string) GatewayResponse {
	sess := store.get(sessionKey)
	if sess == nil {
		return onboardStart(ctx, requestID, sessionKey, daemon, store, actor)
	}
	input := strings.TrimSpace(strings.Join(args, " "))

	switch sess.Step {
	case OnboardIdle:
		return onboardSelectAgent(ctx, requestID, sessionKey, input, daemon, store, actor)
	case OnboardAgentSelected:
		return onboardEnvInput(requestID, sessionKey, input, store)
	case OnboardEnvConfigured:
		return onboardConfirm(ctx, requestID, sessionKey, input, daemon, store, actor)
	case OnboardInstalling:
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: "⏳ Installation is in progress. Please wait..."}
	case OnboardDone:
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: "✅ Onboarding is complete! Run `/onboard` to set up another agent, or `/onboard status` to check."}
	default:
		return errResp(requestID, "E_USAGE", "Unexpected state. Run `/onboard` to start over.")
	}
}

func onboardSelectAgent(ctx context.Context, requestID, sessionKey, agentID string, daemon *DaemonClient, store *OnboardStore, actor string) GatewayResponse {
	if agentID == "" {
		return errResp(requestID, "E_USAGE", "Please provide an agent name. Run `/onboard` to see available agents.")
	}
	agents, err := daemon.ListAgents(ctx, actor, requestID)
	if err != nil {
		return daemonErrResp(requestID, err)
	}
	var found *AgentState
	for i := range agents {
		if agents[i].ID == agentID {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		return errResp(requestID, "E_AGENT_NOT_FOUND", fmt.Sprintf("Agent %q not found. Run `/onboard` to see available agents.", agentID))
	}
	store.update(sessionKey, func(s *OnboardSession) {
		s.Step = OnboardAgentSelected
		s.SelectedAgent = agentID
	})
	lines := []string{
		fmt.Sprintf("Selected agent: **%s** (%s)", found.Name, found.ID),
		"",
		"Provide any environment variables as KEY=VALUE pairs (one per message).",
		"When done, reply with `/onboard done`.",
		"To skip env vars, reply `/onboard done` now.",
	}
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
}

func onboardEnvInput(requestID, sessionKey, input string, store *OnboardStore) GatewayResponse {
	if strings.ToLower(input) == "done" {
		sess := store.update(sessionKey, func(s *OnboardSession) {
			s.Step = OnboardEnvConfigured
		})
		keys := make([]string, 0, len(sess.EnvVars))
		for k := range sess.EnvVars {
			keys = append(keys, k)
		}
		envSummary := "\nNo environment variables set."
		if len(keys) > 0 {
			envSummary = "\nEnvironment variables: " + strings.Join(keys, ", ")
		}
		lines := []string{
			fmt.Sprintf("Ready to install **%s**?%s", sess.SelectedAgent, envSummary),
			"",
			"Reply `/onboard yes` to proceed or `/onboard no` to go back.",
		}
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
	}
	eqIdx := strings.IndexByte(input, '=')
	if eqIdx <= 0 {
		return errResp(requestID, "E_USAGE", "Please provide env vars as KEY=VALUE, or reply `/onboard done` to continue.")
	}
	envName := strings.TrimSpace(input[:eqIdx])
	envValue := strings.TrimSpace(input[eqIdx+1:])
	if envName == "" || envValue == "" {
		return errResp(requestID, "E_USAGE", "Please provide env vars as KEY=VALUE, or reply `/onboard done` to continue.")
	}
	store.update(sessionKey, func(s *OnboardSession) {
		s.EnvVars[envName] = envValue
	})
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: fmt.Sprintf("✅ %s set. Add more variables or reply `/onboard done` to continue.", envName)}
}

func onboardConfirm(ctx context.Context, requestID, sessionKey, input string, daemon *DaemonClient, store *OnboardStore, actor string) GatewayResponse {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "no" || normalized == "back" {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAgentSelected })
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: "Going back. Provide env vars as KEY=VALUE, or reply `/onboard done` to continue."}
	}
	if normalized != "yes" && normalized != "y" {
		return errResp(requestID, "E_USAGE", "Reply `/onboard yes` to install or `/onboard no` to go back.")
	}
	sess := store.get(sessionKey)
	if sess == nil {
		return errResp(requestID, "E_USAGE", "No active session. Run `/onboard` to start.")
	}
	agentID := sess.SelectedAgent
	store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardInstalling })

	if err := daemon.InstallAgent(ctx, agentID, actor, requestID); err != nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
		return daemonErrResp(requestID, err)
	}
	if err := daemon.StartAgent(ctx, agentID, actor, requestID); err != nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
		if de, ok := err.(*DaemonClientError); ok {
			return GatewayResponse{RequestID: requestID, Result: "ok", Message: fmt.Sprintf("%s installed but failed to start: %s", agentID, de.Message)}
		}
		return daemonErrResp(requestID, err)
	}
	statuses, err := daemon.GetStatus(ctx, agentID, actor, requestID)
	health := "unknown"
	if err == nil && len(statuses) > 0 {
		health = statuses[0].Health
	}
	store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
	return GatewayResponse{
		RequestID: requestID,
		Result:    "ok",
		Message:   fmt.Sprintf("🎉 %s installed and running (%s). Onboarding complete!", agentID, health),
	}
}

func onboardCancel(requestID, sessionKey string, store *OnboardStore) GatewayResponse {
	sess := store.get(sessionKey)
	if sess == nil || sess.Step == OnboardIdle || sess.Step == OnboardDone {
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: "No active onboarding session to cancel."}
	}
	store.clear(sessionKey)
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: "🚫 Onboarding cancelled. Run `/onboard` to start again."}
}

func onboardStatus(ctx context.Context, requestID string, daemon *DaemonClient, actor string) GatewayResponse {
	statuses, err := daemon.GetStatus(ctx, "", actor, requestID)
	if err != nil {
		return daemonErrResp(requestID, err)
	}
	if len(statuses) == 0 {
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: "No agents configured."}
	}
	lines := []string{"Agent status:"}
	for _, s := range statuses {
		emoji := "🔴"
		if s.Health == "healthy" {
			emoji = "🟢"
		} else if s.Runtime == "stopped" {
			emoji = "⚪"
		}
		lines = append(lines, fmt.Sprintf("%s %s (%s): %s, %s, health=%s", emoji, s.Name, s.ID, s.InstallState, s.Runtime, s.Health))
	}
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
}
