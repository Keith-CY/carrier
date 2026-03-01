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
	// BaseURL is an optional endpoint override for compatible providers.
	BaseURL string
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
		if strings.TrimSpace(p.BaseURLEnv) != "" {
			if strings.TrimSpace(p.DefaultBase) == "" {
				return fmt.Sprintf(
					"Please paste your API key for **%s** (env: `%s`).\n\n"+
						"Then provide the base URL (env: `%s`), e.g.:\n"+
						"  https://your-endpoint.example.com/v1\n\n"+
						"Reply with: <api-key> or KEY=<api-key> URL=<base-url>\n\n"+
						"Tip: reply `/onboard reuse` to reuse a credential saved by Carrier.",
					p.Name, p.EnvVar, p.BaseURLEnv,
				)
			}
			return fmt.Sprintf(
				"Please paste your API key for **%s** (env: `%s`).\n\n"+
					"Default base URL: %s\n"+
					"To override, reply: KEY=<api-key> URL=<custom-url>\n\n"+
					"Tip: reply `/onboard reuse` to reuse a credential saved by Carrier.",
				p.Name, p.EnvVar, p.DefaultBase,
			)
		}
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
		if strings.TrimSpace(p.BaseURLEnv) != "" {
			if strings.TrimSpace(p.DefaultBase) != "" {
				return fmt.Sprintf(
					"**%s** requires no API key.\n\n"+
						"Default endpoint: %s\n"+
						"To use a custom endpoint, reply: URL=<your-url>\n"+
						"Or reply `/onboard confirm` to use the default.",
					p.Name,
					p.DefaultBase,
				)
			}
			return fmt.Sprintf(
				"**%s** requires no API key.\n\n"+
					"Provide endpoint URL with: URL=<your-url>\n"+
					"Then reply `/onboard confirm`.",
				p.Name,
			)
		}
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
		keyValue, baseURL, parseErr := parseKeyAndURLInput(input)
		if parseErr != nil {
			return nil, parseErr
		}
		if strings.Contains(input, "KEY=") || strings.Contains(input, "URL=") {
			if strings.TrimSpace(keyValue) == "" {
				return nil, fmt.Errorf("KEY is required for %s", p.Name)
			}
		}
		if keyValue != "" {
			input = keyValue
		}
		result, err := handlePastedCredentialInput(p, input, lower, "API key")
		if err != nil {
			return nil, err
		}
		result.BaseURL = strings.TrimSpace(baseURL)
		return result, nil

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
		_, baseURL, parseErr := parseKeyAndURLInput(input)
		if parseErr != nil {
			return nil, parseErr
		}
		if strings.TrimSpace(baseURL) != "" {
			return &ProviderAuthResult{
				Done:         true,
				Instructions: fmt.Sprintf("Saved endpoint override for **%s**.", p.Name),
				BaseURL:      strings.TrimSpace(baseURL),
			}, nil
		}
		if strings.TrimSpace(p.BaseURLEnv) != "" && strings.TrimSpace(p.DefaultBase) == "" {
			if lower == "confirm" || lower == "done" || lower == "yes" || lower == "y" {
				return nil, fmt.Errorf("reply with URL=<your-url> for %s", p.Name)
			}
		}
		return &ProviderAuthResult{
			Done: true,
		}, nil

	default:
		return &ProviderAuthResult{Done: true}, nil
	}
}

func parseKeyAndURLInput(input string) (keyValue string, baseURL string, err error) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return "", "", nil
	}
	var explicit bool
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "KEY="):
			explicit = true
			keyValue = strings.TrimSpace(strings.TrimPrefix(part, "KEY="))
		case strings.HasPrefix(part, "URL="):
			explicit = true
			baseURL = strings.TrimSpace(strings.TrimPrefix(part, "URL="))
		}
	}
	if !explicit {
		return "", "", nil
	}
	if keyValue == "" && baseURL == "" {
		return "", "", fmt.Errorf("auth input was parsed but neither KEY nor URL had a value")
	}
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return "", "", fmt.Errorf("URL must start with http:// or https://")
	}
	return keyValue, baseURL, nil
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
func ProviderEnvVarsToSet(p *LLMProvider, value, baseURL string) map[string]string {
	if p == nil {
		return nil
	}
	out := map[string]string{}
	trimmed := strings.TrimSpace(value)
	if strings.TrimSpace(p.EnvVar) != "" && trimmed != "" {
		out[p.EnvVar] = trimmed
	}
	if strings.TrimSpace(p.BaseURLEnv) != "" {
		resolvedBase := strings.TrimSpace(baseURL)
		if resolvedBase == "" {
			resolvedBase = strings.TrimSpace(p.DefaultBase)
		}
		if resolvedBase != "" {
			out[p.BaseURLEnv] = resolvedBase
		}
	}
	// Compatibility alias:
	// some downstream runtimes (including older PicoClaw flows) still
	// resolve OpenAI-compatible credentials from OPENAI_API_KEY.
	if strings.EqualFold(strings.TrimSpace(p.ID), "openai-codex") {
		out["OPENAI_API_KEY"] = trimmed
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
