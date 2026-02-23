package gateway

import (
	"carrier/daemon/internal/config"
	"strings"
)

func readCarrierDefaultProviderID() string {
	cfg, err := config.LoadCarrierDefaultModel()
	if err != nil || cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.ProviderID)
}
