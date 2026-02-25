package gateway

import "net/http"

type gatewayFeatureFlags struct {
	RemoteControlPlaneEnabled bool `json:"remoteControlPlaneEnabled"`
	RemoteChatEnabled         bool `json:"remoteChatEnabled"`
	ProviderBindingEnabled    bool `json:"providerBindingEnabled"`
}

func handleFeatureFlags(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	flags := effectiveGatewayFeatureFlags(cfg)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"features":  flags,
	})
}
