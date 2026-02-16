package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
)

func TestParseAgentActionPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantID     string
		wantAction string
		wantOK     bool
	}{
		// Valid cases
		{name: "valid", path: "/api/v1/agents/myagent/status", wantID: "myagent", wantAction: "status", wantOK: true},
		{name: "dots_hyphens_underscores", path: "/api/v1/agents/my.agent_1-2/logs", wantID: "my.agent_1-2", wantAction: "logs", wantOK: true},

		// Missing agent ID
		{name: "no_id_no_action", path: "/api/v1/agents/", wantOK: false},
		{name: "only_action", path: "/api/v1/agents//status", wantOK: false},

		// Missing action
		{name: "id_no_action", path: "/api/v1/agents/myagent", wantOK: false},
		{name: "id_trailing_slash", path: "/api/v1/agents/myagent/", wantOK: false},

		// Extra path segments
		{name: "extra_segment", path: "/api/v1/agents/myagent/status/extra", wantOK: false},
		{name: "deep_nesting", path: "/api/v1/agents/myagent/status/extra/deep", wantOK: false},

		// Empty segments (double slashes)
		{name: "double_slash_in_id", path: "/api/v1/agents//myagent/status", wantOK: false},
		{name: "double_slash_after_action", path: "/api/v1/agents/myagent//status", wantOK: false},

		// Special characters in agent ID
		{name: "slash_in_id", path: "/api/v1/agents/my%2Fagent/status", wantOK: false},
		{name: "backslash_in_id", path: "/api/v1/agents/my%5Cagent/status", wantOK: false},
		{name: "dot_dot_traversal", path: "/api/v1/agents/..%2F..%2Fetc/status", wantOK: false},
		{name: "dot_dot_in_id", path: "/api/v1/agents/a..b/status", wantOK: false},
		{name: "leading_dot", path: "/api/v1/agents/.hidden/status", wantOK: false},
		{name: "leading_hyphen", path: "/api/v1/agents/-bad/status", wantOK: false},
		{name: "space_in_id", path: "/api/v1/agents/my%20agent/status", wantOK: false},
		{name: "unicode_in_id", path: "/api/v1/agents/ag%C3%A9nt/status", wantOK: false},

		// Wrong prefix
		{name: "wrong_prefix", path: "/api/v2/agents/myagent/status", wantOK: false},
		{name: "no_prefix", path: "/agents/myagent/status", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, action, ok := parseAgentActionPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("parseAgentActionPath(%q): ok=%v, want %v (id=%q action=%q)", tt.path, ok, tt.wantOK, id, action)
			}
			if ok {
				if id != tt.wantID {
					t.Errorf("id=%q, want %q", id, tt.wantID)
				}
				if action != tt.wantAction {
					t.Errorf("action=%q, want %q", action, tt.wantAction)
				}
			}
		})
	}
}

func TestAgentsPathHTTPRouting(t *testing.T) {
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		// Valid agent + valid action → 200 (openclaw is registered)
		{name: "valid_status", method: "GET", path: "/api/v1/agents/openclaw/status", wantStatus: http.StatusOK},
		{name: "valid_logs", method: "GET", path: "/api/v1/agents/openclaw/logs", wantStatus: http.StatusOK},

		// Missing agent ID → 404
		{name: "bare_trailing_slash", method: "GET", path: "/api/v1/agents/", wantStatus: http.StatusNotFound},

		// Agent ID only, no action → 404
		{name: "id_no_action", method: "GET", path: "/api/v1/agents/openclaw", wantStatus: http.StatusNotFound},

		// Extra path segments → 404
		{name: "extra_segments", method: "GET", path: "/api/v1/agents/openclaw/status/extra", wantStatus: http.StatusNotFound},

		// Unknown action → 404
		{name: "unknown_action", method: "GET", path: "/api/v1/agents/openclaw/unknown", wantStatus: http.StatusNotFound},

		// Empty ID segment (double slash) → 301 (ServeMux cleans path)
		{name: "empty_id_segment", method: "GET", path: "/api/v1/agents//status", wantStatus: http.StatusMovedPermanently},

		// Special characters → 301 for traversal (ServeMux cleans ..), 404 for others
		{name: "traversal_id", method: "GET", path: "/api/v1/agents/..%2F..%2Fetc/status", wantStatus: http.StatusMovedPermanently},
		{name: "slash_encoded_id", method: "GET", path: "/api/v1/agents/a%2Fb/status", wantStatus: http.StatusNotFound},
		{name: "leading_dot_id", method: "GET", path: "/api/v1/agents/.hidden/status", wantStatus: http.StatusNotFound},

		// Non-existent agent, valid format → 404 (from service)
		{name: "nonexistent_agent", method: "GET", path: "/api/v1/agents/doesnotexist/status", wantStatus: http.StatusNotFound},

		// /api/v1/agents/status special case → 200
		{name: "all_agents_status", method: "GET", path: "/api/v1/agents/status", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("%s %s: got %d, want %d; body: %s",
					tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestParsePathAgentIDMalformed(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: true},
		{name: "valid", raw: "myagent", wantErr: false},
		{name: "with_slash", raw: "foo/bar", wantErr: true},
		{name: "with_backslash", raw: "foo\\bar", wantErr: true},
		{name: "dot_dot", raw: "..", wantErr: true},
		{name: "encoded_slash", raw: "foo%2Fbar", wantErr: true},
		{name: "leading_dot", raw: ".hidden", wantErr: true},
		{name: "leading_hyphen", raw: "-agent", wantErr: true},
		{name: "space", raw: "my agent", wantErr: true},
		{name: "valid_complex", raw: "agent-1.0_test", wantErr: false},
		{name: "invalid_percent_encoding", raw: "%zz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePathAgentID(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePathAgentID(%q): err=%v, wantErr=%v", tt.raw, err, tt.wantErr)
			}
		})
	}
}
