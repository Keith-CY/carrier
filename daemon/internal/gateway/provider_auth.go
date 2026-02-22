package gateway

import (
	"fmt"
	"strings"
)

// ProviderAuthResult holds the result of a provider auth interaction.
type ProviderAuthResult struct {
	// EnvVar is the environment variable name to set.
	EnvVar string
	// Value is the value to set for EnvVar.
	Value string
	// Instructions is a human-readable message to show the user (for non-API-key flows).
	Instructions string
	// Done reports whether auth is complete (true for api_key / none / instructions-only modes).
	Done bool
}

// BuildProviderAuthPrompt returns the prompt to show the user when requesting credentials
// for the given provider.
func BuildProviderAuthPrompt(p *LLMProvider) string {
	switch p.AuthMode {
	case AuthModeAPIKey:
		return fmt.Sprintf(
			"Please paste your API key for **%s** (env: `%s`).\n\n"+
				"Tip: reply `/onboard reuse` to reuse a credential saved by Carrier.",
			p.Name, p.EnvVar,
		)
	case AuthModeOAuthDeviceCode:
		if strings.TrimSpace(p.EnvVar) == "" {
			return fmt.Sprintf(
				"**%s** uses an OAuth device-code flow.\n\n"+
					"Complete auth in your tool, then reply `/onboard confirm` when done.",
				p.Name,
			)
		}
		return fmt.Sprintf(
			"**%s** uses an OAuth device-code flow.\n\n"+
				"Complete the device-code login in your auth tool/browser, then paste `%s` here.\n\n"+
				"Tip: reply `/onboard reuse` to reuse a credential saved by Carrier.",
			p.Name, p.EnvVar,
		)
	case AuthModeOAuthPlugin:
		return fmt.Sprintf(
			"**%s** uses an OAuth plugin flow.\n\n"+
				"Complete auth in the provider plugin/CLI, then reply `/onboard confirm` when done.",
			p.Name,
		)
	case AuthModeGcloudADC:
		return fmt.Sprintf(
			"**%s** uses Google Application Default Credentials.\n\n"+
				"Ensure you have `gcloud` authenticated:\n\n"+
				"```\ngcloud auth application-default login\n```\n\n"+
				"Then reply `/onboard confirm` when done.",
			p.Name,
		)
	case AuthModeNone:
		return fmt.Sprintf("**%s** requires no authentication. Proceeding automatically.", p.Name)
	default:
		return fmt.Sprintf("Provider **%s** auth mode is not recognised. Reply `/onboard confirm` to continue.", p.Name)
	}
}

// HandleProviderAuthInput processes user input during the auth_configured step.
// It returns a ProviderAuthResult describing what happened.
func HandleProviderAuthInput(p *LLMProvider, input string) (*ProviderAuthResult, error) {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)

	switch p.AuthMode {
	case AuthModeAPIKey:
		return handlePastedCredentialInput(p, input, lower, "API key")

	case AuthModeOAuthDeviceCode:
		// Prefer token paste so Carrier can persist and later reuse credentials.
		if strings.TrimSpace(p.EnvVar) == "" {
			if lower == "confirm" || lower == "done" || lower == "yes" || lower == "y" {
				return &ProviderAuthResult{Done: true}, nil
			}
			return nil, fmt.Errorf("reply `/onboard confirm` once you have completed the auth steps shown above")
		}
		if lower == "confirm" || lower == "done" || lower == "yes" || lower == "y" {
			return nil, fmt.Errorf("please paste `%s` after device-code login, or reply `/onboard reuse`", p.EnvVar)
		}
		return handlePastedCredentialInput(p, input, lower, p.EnvVar+" token")

	case AuthModeOAuthPlugin, AuthModeGcloudADC:
		// These require external tooling. We accept "confirm" / "done" / "yes" to proceed.
		if lower == "confirm" || lower == "done" || lower == "yes" || lower == "y" {
			return &ProviderAuthResult{
				Done: true,
			}, nil
		}
		return nil, fmt.Errorf("reply `/onboard confirm` once you have completed the auth steps shown above")

	case AuthModeNone:
		// No key needed — auto-complete.
		return &ProviderAuthResult{
			Done: true,
		}, nil

	default:
		return &ProviderAuthResult{Done: true}, nil
	}
}

func handlePastedCredentialInput(p *LLMProvider, input, lowerInput, label string) (*ProviderAuthResult, error) {
	if strings.TrimSpace(p.EnvVar) == "" {
		return nil, fmt.Errorf("provider %s does not expose an env var for interactive auth input", p.Name)
	}
	if lowerInput == "reuse" {
		value, backend, ok, err := loadProviderCredential(p.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load saved credential for %s: %w", p.Name, err)
		}
		if !ok {
			return nil, fmt.Errorf("no saved credential found for %s; please paste your %s", p.Name, label)
		}
		return &ProviderAuthResult{
			EnvVar:       p.EnvVar,
			Value:        value,
			Instructions: fmt.Sprintf("Reused saved credential for **%s** from %s.", p.Name, backend),
			Done:         true,
		}, nil
	}

	if input == "" {
		return nil, fmt.Errorf("%s cannot be empty; please paste `%s`", label, p.EnvVar)
	}
	result := &ProviderAuthResult{
		EnvVar: p.EnvVar,
		Value:  input,
		Done:   true,
	}
	backend, err := saveProviderCredential(p.ID, input)
	if err != nil {
		result.Instructions = fmt.Sprintf("Credential applied for this onboarding session, but failed to save: %v", err)
		return result, nil
	}
	result.Instructions = fmt.Sprintf("Credential saved for future reuse (%s).", backend)
	return result, nil
}

// ProviderEnvVarsToSet returns the env var map entries that should be auto-set for a provider,
// given a credential value. For api_key mode this is {EnvVar: value}; for others it may be empty.
func ProviderEnvVarsToSet(p *LLMProvider, value string) map[string]string {
	if p == nil {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if strings.TrimSpace(p.EnvVar) == "" || trimmed == "" {
		return nil
	}
	out := map[string]string{p.EnvVar: trimmed}
	// Compatibility alias:
	// some downstream runtimes (including older PicoClaw flows) still
	// resolve OpenAI-compatible credentials from OPENAI_API_KEY.
	if strings.EqualFold(strings.TrimSpace(p.ID), "openai-codex") {
		out["OPENAI_API_KEY"] = trimmed
	}
	return out
}
