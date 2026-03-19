package gateway

import (
	"context"
	"fmt"
	"log"
	"strings"
)

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
	case OnboardChannelSelect:
		return onboardSelectChannel(requestID, sessionKey, input, store)
	case OnboardChannelToken:
		return onboardCaptureChannelToken(requestID, sessionKey, input, store)
	case OnboardAgentSelected:
		return onboardSelectProvider(requestID, sessionKey, input, store)
	case OnboardProviderSelected:
		return onboardHandleAuth(requestID, sessionKey, input, store)
	case OnboardAuthConfigured:
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
		s.SelectedAgent = agentID
		s.SelectedAgentName = found.Name
	})
	if isManagedAgent(agentID) {
		store.update(sessionKey, func(s *OnboardSession) {
			s.Step = OnboardChannelSelect
		})
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: renderManagedChannelPrompt(agentID)}
	}
	store.update(sessionKey, func(s *OnboardSession) {
		s.Step = OnboardAgentSelected
	})

	// Build provider list
	return buildProviderListResponse(requestID, found)
}

func onboardSelectChannel(requestID, sessionKey, input string, store *OnboardStore) GatewayResponse {
	sess := store.get(sessionKey)
	if sess == nil {
		return errResp(requestID, "E_USAGE", "No active session. Run `/onboard` to start.")
	}
	if !isManagedAgent(sess.SelectedAgent) {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAgentSelected })
		return errResp(requestID, "E_USAGE", "Channel selection is only required for PicoClaw/OpenClaw/ZeroClaw in this flow.")
	}
	channel, ok := parseManagedChannel(sess.SelectedAgent, strings.TrimSpace(input))
	if !ok {
		return errResp(requestID, "E_USAGE", "Unsupported channel. Reply `/onboard telegram` to continue.")
	}
	store.update(sessionKey, func(s *OnboardSession) {
		s.SelectedChannel = channel.ID
		s.ChannelToken = ""
		s.ChannelSetupPending = true
		s.Step = OnboardAgentSelected
	})
	name := strings.TrimSpace(sess.SelectedAgentName)
	if name == "" {
		name = managedAgentDisplayName(sess.SelectedAgent)
	}
	if name == "" {
		name = sess.SelectedAgent
	}
	resp := buildProviderListResponse(requestID, &AgentState{ID: sess.SelectedAgent, Name: name})
	resp.Message = strings.TrimSpace(fmt.Sprintf(
		"✅ %s channel selected: **%s** (`%s`).\n\n🔒 Chat onboarding skips bot token input to avoid exposing secrets in chat history.\nConfigure channel token in Web UI after install.\n\n%s",
		managedAgentDisplayName(sess.SelectedAgent),
		channel.Name,
		channel.ID,
		resp.Message,
	))
	return resp
}

func onboardCaptureChannelToken(requestID, sessionKey, input string, store *OnboardStore) GatewayResponse {
	sess := store.get(sessionKey)
	if sess == nil {
		return errResp(requestID, "E_USAGE", "No active session. Run `/onboard` to start.")
	}
	if !isManagedAgent(sess.SelectedAgent) {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAgentSelected })
		return errResp(requestID, "E_USAGE", "Channel token input is only valid for managed agents.")
	}
	store.update(sessionKey, func(s *OnboardSession) {
		s.ChannelToken = ""
		s.ChannelSetupPending = true
		s.Step = OnboardAgentSelected
	})
	name := strings.TrimSpace(sess.SelectedAgentName)
	if name == "" {
		name = sess.SelectedAgent
	}
	agent := &AgentState{ID: sess.SelectedAgent, Name: name}
	resp := buildProviderListResponse(requestID, agent)
	channelID := strings.TrimSpace(sess.SelectedChannel)
	if channelID == "" {
		channelID = "selected channel"
	}
	resp.Message = strings.TrimSpace(fmt.Sprintf(
		"🔒 Bot token entry in chat onboarding is disabled to protect secrets.\nConfigure token for `%s` in Web UI, then continue.\n\n%s",
		channelID,
		resp.Message,
	))
	return resp
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

		channelLine := ""
		if sess.SelectedChannel != "" {
			channelLine = fmt.Sprintf("\nChannel: %s", sess.SelectedChannel)
			if sess.ChannelSetupPending {
				channelLine += " (token setup pending in Web UI)"
			}
		}
		providerLine := ""
		if sess.SelectedProvider != "" {
			providerLine = fmt.Sprintf("\nLLM Provider: %s", sess.SelectedProvider)
		}

		lines := []string{
			fmt.Sprintf("Ready to install **%s**?%s%s%s", sess.SelectedAgent, channelLine, providerLine, envSummary),
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
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAuthConfigured })
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
	setupNotes := []string{}
	if isManagedAgent(agentID) {
		result, err := prepareManagedOnboard(agentID, sess, actor)
		if err != nil {
			store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardEnvConfigured })
			log.Printf("[gateway] onboarding: prepare %s artifacts failed detail=%s", agentID, RedactErrorMessage(err.Error()))
			return errResp(requestID, "E_ENV", fmt.Sprintf("failed to prepare %s onboarding artifacts", agentID))
		}
		setupNotes = append(setupNotes, fmt.Sprintf("%s workspace: %s", managedAgentDisplayName(agentID), result.WorkspacePath))
		setupNotes = append(setupNotes, fmt.Sprintf("%s config: %s", managedAgentDisplayName(agentID), result.ConfigPath))
		setupNotes = append(setupNotes, fmt.Sprintf("Carrier record: %s", result.RecordPath))
		if sess.ChannelSetupPending {
			setupNotes = append(setupNotes, "Channel token setup is pending. Configure it in Web UI before using chat commands.")
		}
	}
	if err := applyOnboardEnvVars(sess.EnvVars); err != nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardEnvConfigured })
		log.Printf("[gateway] onboarding: apply env vars failed detail=%s", RedactErrorMessage(err.Error()))
		return errResp(requestID, "E_ENV", "failed to apply environment variables")
	}

	if err := daemon.InstallAgent(ctx, agentID, actor, requestID); err != nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
		return daemonErrResp(requestID, err)
	}
	if err := daemon.StartAgent(ctx, agentID, actor, requestID); err != nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
		if de, ok := err.(*DaemonClientError); ok {
			_, _, msg := mapDaemonErrorToExternal(de.Code)
			return GatewayResponse{RequestID: requestID, Result: "ok", Message: fmt.Sprintf("%s installed but failed to start: %s", agentID, msg)}
		}
		return daemonErrResp(requestID, err)
	}
	statuses, err := daemon.GetStatus(ctx, agentID, actor, requestID)
	health := "unknown"
	if err == nil && len(statuses) > 0 {
		health = statuses[0].Health
	}
	store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardDone })
	pairHint := ""
	if isPicoclawAgent(agentID) {
		if logs, logErr := daemon.GetLogs(ctx, agentID, 120, actor, requestID); logErr == nil && logs != nil {
			if pairCode := extractPairCode(logs.Lines); pairCode != "" {
				pairHint = fmt.Sprintf("PicoClaw pair code: `%s`. Send `/pair %s` in your PicoClaw Telegram bot.", pairCode, pairCode)
			}
		}
		if pairHint == "" {
			pairHint = "Open your PicoClaw Telegram bot and finish any in-bot pairing/authorization prompts to complete onboarding."
		}
	}
	lines := []string{fmt.Sprintf("🎉 %s installed and running (%s). Onboarding complete!", agentID, health)}
	if len(setupNotes) > 0 {
		lines = append(lines, "")
		lines = append(lines, setupNotes...)
	}
	if pairHint != "" {
		lines = append(lines, "")
		lines = append(lines, pairHint)
	}
	return GatewayResponse{
		RequestID: requestID,
		Result:    "ok",
		Message:   strings.Join(lines, "\n"),
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
