package gateway

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type GatewayRole string

const (
	GatewayRoleViewer   GatewayRole = "viewer"
	GatewayRoleOperator GatewayRole = "operator"
	GatewayRoleApprover GatewayRole = "approver"
	GatewayRoleAdmin    GatewayRole = "admin"
)

type gatewayPermissionSet struct {
	ViewExecutions    bool `json:"viewExecutions"`
	LaunchExecutions  bool `json:"launchExecutions"`
	ApproveExecutions bool `json:"approveExecutions"`
	ManagePolicies    bool `json:"managePolicies"`
	ManageProviders   bool `json:"manageProviders"`
	ManageHosts       bool `json:"manageHosts"`
}

type gatewayAuthzState struct {
	Role        GatewayRole          `json:"role"`
	Permissions gatewayPermissionSet `json:"permissions"`
}

func normalizeGatewayRole(raw string) GatewayRole {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(GatewayRoleViewer):
		return GatewayRoleViewer
	case string(GatewayRoleOperator):
		return GatewayRoleOperator
	case string(GatewayRoleApprover):
		return GatewayRoleApprover
	case string(GatewayRoleAdmin):
		return GatewayRoleAdmin
	default:
		return GatewayRoleAdmin
	}
}

func permissionsForGatewayRole(role GatewayRole) gatewayPermissionSet {
	switch normalizeGatewayRole(string(role)) {
	case GatewayRoleViewer:
		return gatewayPermissionSet{
			ViewExecutions: true,
		}
	case GatewayRoleOperator:
		return gatewayPermissionSet{
			ViewExecutions:   true,
			LaunchExecutions: true,
		}
	case GatewayRoleApprover:
		return gatewayPermissionSet{
			ViewExecutions:    true,
			ApproveExecutions: true,
		}
	default:
		return gatewayPermissionSet{
			ViewExecutions:    true,
			LaunchExecutions:  true,
			ApproveExecutions: true,
			ManagePolicies:    true,
			ManageProviders:   true,
			ManageHosts:       true,
		}
	}
}

func canViewExecutions(role GatewayRole) bool {
	return permissionsForGatewayRole(role).ViewExecutions
}

func canLaunchExecutions(role GatewayRole) bool {
	return permissionsForGatewayRole(role).LaunchExecutions
}

func canApproveExecutions(role GatewayRole) bool {
	return permissionsForGatewayRole(role).ApproveExecutions
}

func canManagePolicies(role GatewayRole) bool {
	return permissionsForGatewayRole(role).ManagePolicies
}

func canManageProviders(role GatewayRole) bool {
	return permissionsForGatewayRole(role).ManageProviders
}

func canManageHosts(role GatewayRole) bool {
	return permissionsForGatewayRole(role).ManageHosts
}

func gatewayAuthzForRole(role GatewayRole) gatewayAuthzState {
	normalized := normalizeGatewayRole(string(role))
	return gatewayAuthzState{
		Role:        normalized,
		Permissions: permissionsForGatewayRole(normalized),
	}
}

func gatewayTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		if token := strings.TrimSpace(after); token != "" {
			return token
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func resolveGatewayRoleForToken(token string, cfg *GatewayConfig) (GatewayRole, bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	if cfg != nil {
		if role, ok := cfg.RoleTokens[trimmed]; ok {
			return normalizeGatewayRole(string(role)), true
		}
		if strings.TrimSpace(cfg.APIToken) != "" && subtleTokenCompare(trimmed, cfg.APIToken) {
			return GatewayRoleAdmin, true
		}
	}
	return "", false
}

func subtleTokenCompare(leftToken, rightToken string) bool {
	left := sha256.Sum256([]byte(strings.TrimSpace(leftToken)))
	right := sha256.Sum256([]byte(strings.TrimSpace(rightToken)))
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func resolveGatewayRoleFromRequest(r *http.Request, cfg *GatewayConfig) (GatewayRole, *apiErr) {
	if cfg == nil {
		return GatewayRoleAdmin, nil
	}
	authRequired := strings.TrimSpace(cfg.APIToken) != "" || len(cfg.RoleTokens) > 0
	if !authRequired {
		return GatewayRoleAdmin, nil
	}
	token := gatewayTokenFromRequest(r)
	if token == "" {
		return "", &apiErr{code: "E_GATEWAY_AUTH_REQUIRED", msg: "gateway api token required"}
	}
	role, ok := resolveGatewayRoleForToken(token, cfg)
	if !ok {
		return "", &apiErr{code: "E_GATEWAY_AUTH_INVALID", msg: "invalid gateway api token"}
	}
	return role, nil
}

func checkGatewayAccess(r *http.Request, cfg *GatewayConfig) *apiErr {
	_, err := resolveGatewayRoleFromRequest(r, cfg)
	return err
}

func requireGatewayPermission(w http.ResponseWriter, r *http.Request, cfg *GatewayConfig, check func(GatewayRole) bool, code, message string) (GatewayRole, bool) {
	role, err := resolveGatewayRoleFromRequest(r, cfg)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody(err.code, err.msg))
		return "", false
	}
	if !check(role) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"result": "error",
			"error": map[string]interface{}{
				"code":    strings.TrimSpace(code),
				"message": strings.TrimSpace(message),
			},
			"authz": gatewayAuthzForRole(role),
		})
		return role, false
	}
	return role, true
}
