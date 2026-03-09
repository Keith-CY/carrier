package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRBACPermissions(t *testing.T) {
	cases := []struct {
		role GatewayRole
		want gatewayPermissionSet
	}{
		{
			role: GatewayRoleViewer,
			want: gatewayPermissionSet{
				ViewExecutions:   true,
				LaunchExecutions: false,
				ApproveExecutions: false,
				ManagePolicies:   false,
				ManageProviders:  false,
				ManageHosts:      false,
			},
		},
		{
			role: GatewayRoleOperator,
			want: gatewayPermissionSet{
				ViewExecutions:   true,
				LaunchExecutions: true,
				ApproveExecutions: false,
				ManagePolicies:   false,
				ManageProviders:  false,
				ManageHosts:      false,
			},
		},
		{
			role: GatewayRoleApprover,
			want: gatewayPermissionSet{
				ViewExecutions:   true,
				LaunchExecutions: false,
				ApproveExecutions: true,
				ManagePolicies:   false,
				ManageProviders:  false,
				ManageHosts:      false,
			},
		},
		{
			role: GatewayRoleAdmin,
			want: gatewayPermissionSet{
				ViewExecutions:   true,
				LaunchExecutions: true,
				ApproveExecutions: true,
				ManagePolicies:   true,
				ManageProviders:  true,
				ManageHosts:      true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			got := permissionsForGatewayRole(tc.role)
			if got != tc.want {
				t.Fatalf("permissionsForGatewayRole(%q) = %+v want %+v", tc.role, got, tc.want)
			}
		})
	}
}

func TestRBACMiddleware(t *testing.T) {
	mux := buildFeaturesMux(t, &GatewayConfig{
		APIToken:                  "admin-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		RoleTokens: map[string]GatewayRole{
			"viewer-token":   GatewayRoleViewer,
			"operator-token": GatewayRoleOperator,
			"approver-token": GatewayRoleApprover,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	req.Header.Set("Authorization", "Bearer viewer-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer features status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Authz struct {
			Role        string `json:"role"`
			Permissions struct {
				ViewExecutions    bool `json:"viewExecutions"`
				LaunchExecutions  bool `json:"launchExecutions"`
				ApproveExecutions bool `json:"approveExecutions"`
				ManagePolicies    bool `json:"managePolicies"`
				ManageProviders   bool `json:"manageProviders"`
				ManageHosts       bool `json:"manageHosts"`
			} `json:"permissions"`
		} `json:"authz"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode features authz: %v body=%s", err, rec.Body.String())
	}
	if payload.Authz.Role != string(GatewayRoleViewer) {
		t.Fatalf("authz.role=%q want %q", payload.Authz.Role, GatewayRoleViewer)
	}
	if !payload.Authz.Permissions.ViewExecutions || payload.Authz.Permissions.ManagePolicies || payload.Authz.Permissions.LaunchExecutions {
		t.Fatalf("unexpected viewer permissions: %+v", payload.Authz.Permissions)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status=%d body=%s", rec.Code, rec.Body.String())
	}
}
