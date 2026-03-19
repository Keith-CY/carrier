package gateway

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// buildProviderListResponse constructs the provider selection prompt.
func buildProviderListResponse(requestID string, agent *AgentState) GatewayResponse {
	byCategory := LLMProvidersByCategory()

	lines := []string{
		fmt.Sprintf("Selected agent: **%s** (%s)", agent.Name, agent.ID),
		"",
		"**Step 2 — Choose an LLM Provider**",
		"",
	}

	categoryOrder := []struct{ key, label string }{
		{"builtin", "☁️  Built-in (API key)"},
		{"custom", "🔐 Custom / OAuth"},
		{"compatible", "🔌 OpenAI-Compatible"},
		{"generic", "🖥️  Generic"},
	}

	for _, cat := range categoryOrder {
		providers := byCategory[cat.key]
		if len(providers) == 0 {
			continue
		}
		lines = append(lines, "**"+cat.label+"**")
		for _, p := range providers {
			authBadge := authModeBadge(p.AuthMode)
			lines = append(lines, fmt.Sprintf("  • `%s` — %s %s", p.ID, p.Name, authBadge))
		}
		lines = append(lines, "")
	}

	lines = append(lines, "Reply with a provider ID (e.g. `/onboard anthropic`) to continue,")
	lines = append(lines, "or reply `/onboard skip` to skip provider selection.")
	lines = append(lines, "or reply `/onboard reuse` to reuse Carrier default provider and saved credential.")
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
}

func authModeBadge(m AuthMode) string {
	switch m {
	case AuthModeAPIKey:
		return "[API key]"
	case AuthModeOAuthDeviceCode:
		return "[OAuth device code]"
	case AuthModeOAuthPlugin:
		return "[OAuth plugin]"
	case AuthModeGcloudADC:
		return "[gcloud ADC]"
	case AuthModeNone:
		return "[no auth]"
	default:
		return ""
	}
}

func onboardSelectProvider(requestID, sessionKey, input string, store *OnboardStore) GatewayResponse {
	lower := strings.ToLower(strings.TrimSpace(input))

	if lower == "reuse" {
		defaultProviderID := detectCarrierDefaultProviderID()
		if defaultProviderID == "" {
			return errResp(requestID, "E_PROVIDER_NOT_FOUND", "No Carrier default provider found. Select a provider ID explicitly.")
		}
		p := GetLLMProvider(defaultProviderID)
		if p == nil {
			return errResp(requestID, "E_PROVIDER_NOT_FOUND", fmt.Sprintf("Carrier default provider %q is not available.", defaultProviderID))
		}
		store.update(sessionKey, func(s *OnboardSession) {
			s.Step = OnboardProviderSelected
			s.SelectedProvider = p.ID
		})
		value, backend, ok, err := loadProviderCredential(p.ID)
		if err != nil {
			log.Printf("[gateway] onboarding: load saved credential failed provider=%s detail=%s", p.ID, RedactErrorMessage(err.Error()))
			return errResp(requestID, "E_AUTH_INPUT", "failed to load saved credential for selected provider")
		}
		if ok && p.EnvVar != "" && strings.TrimSpace(value) != "" {
			store.update(sessionKey, func(s *OnboardSession) {
				for k, v := range ProviderEnvVarsToSet(p, value, "") {
					s.EnvVars[k] = v
				}
				s.Step = OnboardAuthConfigured
			})
			lines := []string{
				fmt.Sprintf("✅ Reused Carrier default provider **%s** (`%s`).", p.Name, p.ID),
				fmt.Sprintf("Credential loaded from %s.", backend),
				"",
			}
			lines = append(lines, onboardEnvVarsPromptLines(store.get(sessionKey))...)
			return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
		}
		if p.AuthMode == AuthModeNone {
			store.update(sessionKey, func(s *OnboardSession) {
				for k, v := range ProviderEnvVarsToSet(p, "", "") {
					s.EnvVars[k] = v
				}
				s.Step = OnboardAuthConfigured
			})
			return onboardPromptEnvVars(requestID, store.get(sessionKey))
		}
		lines := []string{
			fmt.Sprintf("Carrier default provider is **%s** (`%s`), but no saved credential was found.", p.Name, p.ID),
			BuildProviderAuthPrompt(p),
		}
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n\n")}
	}

	if lower == "skip" || lower == "done" {
		store.update(sessionKey, func(s *OnboardSession) {
			s.Step = OnboardAuthConfigured
		})
		return onboardPromptEnvVars(requestID, store.get(sessionKey))
	}

	providerID := lower
	p := GetLLMProvider(providerID)
	if p == nil {
		return errResp(requestID, "E_PROVIDER_NOT_FOUND",
			fmt.Sprintf("Provider %q not found. Reply with a valid provider ID or `/onboard skip` to skip.", input))
	}

	store.update(sessionKey, func(s *OnboardSession) {
		s.Step = OnboardProviderSelected
		s.SelectedProvider = p.ID
	})

	if p.AuthMode == AuthModeNone {
		store.update(sessionKey, func(s *OnboardSession) {
			for k, v := range ProviderEnvVarsToSet(p, "", "") {
				s.EnvVars[k] = v
			}
			s.Step = OnboardAuthConfigured
		})
		lines := []string{
			fmt.Sprintf("✅ Provider **%s** selected — no auth needed.", p.Name),
			"",
		}
		if p.ExampleModel != "" {
			lines = append(lines, fmt.Sprintf("Suggested model: `%s`", p.ExampleModel))
			lines = append(lines, "")
		}
		sess := store.get(sessionKey)
		lines = append(lines, onboardEnvVarsPromptLines(sess)...)
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
	}

	prompt := BuildProviderAuthPrompt(p)
	lines := []string{
		fmt.Sprintf("✅ Provider **%s** selected.", p.Name),
		"",
		prompt,
	}
	if hint := credentialReuseHint(p); hint != "" {
		lines = append(lines, "")
		lines = append(lines, hint)
	}
	if p.ExampleModel != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Suggested model: `%s`", p.ExampleModel))
	}
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
}

func credentialReuseHint(p *LLMProvider) string {
	if p == nil {
		return ""
	}
	_, backend, ok, err := loadProviderCredential(p.ID)
	if err != nil || !ok {
		return ""
	}
	return fmt.Sprintf("Saved credential detected for **%s** (%s). Reply `/onboard reuse` to use it.", p.Name, backend)
}

func onboardHandleAuth(requestID, sessionKey, input string, store *OnboardStore) GatewayResponse {
	sess := store.get(sessionKey)
	if sess == nil {
		return errResp(requestID, "E_USAGE", "No active session. Run `/onboard` to start.")
	}

	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "skip" {
		store.update(sessionKey, func(s *OnboardSession) {
			s.Step = OnboardAuthConfigured
		})
		return onboardPromptEnvVars(requestID, store.get(sessionKey))
	}

	p := GetLLMProvider(sess.SelectedProvider)
	if p == nil {
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAuthConfigured })
		return onboardPromptEnvVars(requestID, store.get(sessionKey))
	}

	result, err := HandleProviderAuthInput(p, input)
	if err != nil {
		return errResp(requestID, "E_AUTH_INPUT", err.Error())
	}

	if result.Done {
		store.update(sessionKey, func(s *OnboardSession) {
			for k, v := range ProviderEnvVarsToSet(p, result.Value, result.BaseURL) {
				s.EnvVars[k] = v
			}
		})
		store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAuthConfigured })
		sess = store.get(sessionKey)

		lines := []string{"✅ Authentication configured.", ""}
		if strings.TrimSpace(result.Instructions) != "" {
			lines = append(lines, result.Instructions)
			lines = append(lines, "")
		}
		lines = append(lines, onboardEnvVarsPromptLines(sess)...)
		return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(lines, "\n")}
	}

	return GatewayResponse{RequestID: requestID, Result: "ok", Message: result.Instructions}
}

func onboardPromptEnvVars(requestID string, sess *OnboardSession) GatewayResponse {
	if sess == nil {
		return errResp(requestID, "E_USAGE", "No active session.")
	}
	return GatewayResponse{RequestID: requestID, Result: "ok", Message: strings.Join(onboardEnvVarsPromptLines(sess), "\n")}
}

func onboardEnvVarsPromptLines(sess *OnboardSession) []string {
	lines := []string{
		"**Step 3 — Environment Variables**",
		"",
		"Provide any additional environment variables as KEY=VALUE pairs (one per message).",
		"When done, reply with `/onboard done`.",
		"To skip env vars, reply `/onboard done` now.",
	}
	if len(sess.EnvVars) > 0 {
		keys := make([]string, 0, len(sess.EnvVars))
		for k := range sess.EnvVars {
			keys = append(keys, k)
		}
		lines = append(lines, "")
		lines = append(lines, "Already set: "+strings.Join(keys, ", "))
	}
	return lines
}

func detectCarrierDefaultProviderID() string {
	return strings.TrimSpace(readCarrierDefaultProviderID())
}

func applyOnboardEnvVars(envVars map[string]string) error {
	for k, v := range envVars {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if err := os.Setenv(key, v); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}
