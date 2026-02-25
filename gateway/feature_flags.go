package gateway

import "log"

// effectiveGatewayFeatureFlags returns dependency-normalized flags used by runtime handlers.
//
// Dependency rule (rollout safeguard):
// - remote chat and provider binding are only active when remote control plane is enabled.
func effectiveGatewayFeatureFlags(cfg *GatewayConfig) gatewayFeatureFlags {
	if cfg == nil {
		return gatewayFeatureFlags{}
	}
	remoteControlEnabled := cfg.RemoteControlPlaneEnabled
	return gatewayFeatureFlags{
		RemoteControlPlaneEnabled: remoteControlEnabled,
		RemoteChatEnabled:         remoteControlEnabled && cfg.RemoteChatEnabled,
		ProviderBindingEnabled:    remoteControlEnabled && cfg.ProviderBindingEnabled,
	}
}

// normalizeGatewayConfigFeatureFlags mutates cfg to satisfy feature dependency constraints.
func normalizeGatewayConfigFeatureFlags(cfg *GatewayConfig) {
	if cfg == nil {
		return
	}
	if !cfg.RemoteControlPlaneEnabled {
		if cfg.RemoteChatEnabled {
			log.Println("[gateway] rollout safeguard: disabling remote chat because remote control plane is disabled")
		}
		if cfg.ProviderBindingEnabled {
			log.Println("[gateway] rollout safeguard: disabling provider binding because remote control plane is disabled")
		}
		cfg.RemoteChatEnabled = false
		cfg.ProviderBindingEnabled = false
	}
}
