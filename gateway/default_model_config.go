package gateway

import (
	"carrier/shared/config"
	"strings"
)

func readCarrierDefaultProviderID() string {
	cfg, err := config.LoadCarrierDefaultModel()
	if err != nil || cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.ProviderID)
}
