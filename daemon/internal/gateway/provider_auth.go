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
		return fmt.Sprintf("Please paste your API key for **%s** (env: `%s`):", p.Name, p.EnvVar)
	case AuthModeOAuthDeviceCode:
		return fmt.Sprintf(
			"**%s** uses an OAuth device-code flow.\n\n"+
				"Run the following command to authenticate, then reply `/onboard confirm` when done:\n\n"+
				"```\nopenclaw models auth login --provider %s\n```",
			p.Name, p.ID,
		)
	case AuthModeOAuthPlugin:
		return fmt.Sprintf(
			"**%s** uses an OAuth plugin flow.\n\n"+
				"Run the following command to authenticate, then reply `/onboard confirm` when done:\n\n"+
				"```\nopenclaw models auth login --provider %s\n```",
			p.Name, p.ID,
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

	switch p.AuthMode {
	case AuthModeAPIKey:
		if input == "" {
			return nil, fmt.Errorf("API key cannot be empty; please paste your %s key", p.EnvVar)
		}
		return &ProviderAuthResult{
			EnvVar: p.EnvVar,
			Value:  input,
			Done:   true,
		}, nil

	case AuthModeNone:
		// No key needed — auto-complete.
		return &ProviderAuthResult{
			Done: true,
		}, nil

	case AuthModeOAuthDeviceCode, AuthModeOAuthPlugin, AuthModeGcloudADC:
		// These require external tooling. We accept "confirm" / "done" / "yes" to proceed.
		lower := strings.ToLower(input)
		if lower == "confirm" || lower == "done" || lower == "yes" || lower == "y" {
			return &ProviderAuthResult{
				Done: true,
			}, nil
		}
		return nil, fmt.Errorf("reply `/onboard confirm` once you have completed the auth steps shown above")

	default:
		return &ProviderAuthResult{Done: true}, nil
	}
}

// ProviderEnvVarsToSet returns the env var map entries that should be auto-set for a provider,
// given a credential value. For api_key mode this is {EnvVar: value}; for others it may be empty.
func ProviderEnvVarsToSet(p *LLMProvider, value string) map[string]string {
	if p.AuthMode == AuthModeAPIKey && p.EnvVar != "" && value != "" {
		return map[string]string{p.EnvVar: value}
	}
	return nil
}
