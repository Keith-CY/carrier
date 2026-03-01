package gateway

import "carrier/shared/catalog"

// AuthMode describes how an LLM provider authenticates.
type AuthMode = catalog.AuthMode

const (
	AuthModeAPIKey          AuthMode = catalog.AuthModeAPIKey
	AuthModeOAuthDeviceCode AuthMode = catalog.AuthModeOAuthDeviceCode
	AuthModeOAuthPlugin     AuthMode = catalog.AuthModeOAuthPlugin
	AuthModeGcloudADC       AuthMode = catalog.AuthModeGcloudADC
	AuthModeNone            AuthMode = catalog.AuthModeNone
)

// LLMProvider describes an LLM provider in the catalog.
type LLMProvider = catalog.ProviderSpec

// ListLLMProviders returns a copy of the full provider catalog.
func ListLLMProviders() []LLMProvider {
	return catalog.ListProviders()
}

// GetLLMProvider returns the provider with the given ID, or nil if not found.
func GetLLMProvider(id string) *LLMProvider {
	provider := catalog.GetProvider(id)
	if provider == nil {
		return nil
	}
	cp := *provider
	return &cp
}

// LLMProvidersByCategory returns providers grouped by category.
// Categories are returned in order: builtin, custom, compatible, generic.
func LLMProvidersByCategory() map[string][]LLMProvider {
	return catalog.ProvidersByCategory()
}
