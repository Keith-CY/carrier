package gateway

import (
	"os"
	"strings"
)

func buildCarrierDefaultProviderInfo() map[string]interface{} {
	info := map[string]interface{}{
		"configured": false,
		"available":  false,
		"reusable":   false,
	}

	providerID := strings.TrimSpace(os.Getenv("CARRIER_DEFAULT_PROVIDER_ID"))
	if providerID == "" {
		return info
	}

	info["configured"] = true
	info["id"] = providerID

	provider := GetLLMProvider(providerID)
	if provider == nil {
		info["reason"] = "default provider is not in gateway catalog"
		return info
	}

	info["available"] = true
	info["provider"] = provider

	if provider.AuthMode == AuthModeNone {
		info["has_saved_credential"] = true
		info["credential_backend"] = "none"
		info["reusable"] = true
		return info
	}

	_, backend, hasSaved, err := loadProviderCredential(provider.ID)
	if err != nil {
		info["has_saved_credential"] = false
		info["reusable"] = false
		info["reason"] = "failed to read saved credential"
		info["credential_error"] = err.Error()
		return info
	}
	info["has_saved_credential"] = hasSaved
	if hasSaved {
		info["reusable"] = true
		info["credential_backend"] = backend
	} else {
		info["reusable"] = false
		info["reason"] = "no saved credential"
	}
	return info
}
