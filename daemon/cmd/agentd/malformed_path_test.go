package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
)

// TestParseAgentActionPath_Malformed verifies that parseAgentActionPath rejects
// every known category of malformed input and returns ok == false.
func TestParseAgentActionPath_Malformed(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		// Double slashes
		{"double slash before agent ID", "/api/v1/agents//myagent/start"},
		{"double slash after agent ID", "/api/v1/agents/myagent//start"},

		// Invalid percent-encoding
		{"truncated percent at end", "/api/v1/agents/%2/start"},
		{"truncated percent mid-path", "/api/v1/agents/%2G/start"},
		{"bare percent sign", "/api/v1/agents/%/start"},
		{"null byte encoded", "/api/v1/agents/%00agent/start"},

		// Path traversal attempts
		{"dot-dot agent ID", "/api/v1/agents/../start"},
		{"dot-dot deeper", "/api/v1/agents/../../etc/passwd"},
		{"encoded dot-dot", "/api/v1/agents/%2e%2e/start"},
		{"backslash in agent ID", "/api/v1/agents/foo\\bar/start"},

		// Empty segments / missing components
		{"trailing slash only", "/api/v1/agents/"},
		{"agent ID with trailing slash no action", "/api/v1/agents/myagent/"},
		{"agent ID only no slash", "/api/v1/agents/myagent"},
		{"three segments (extra path)", "/api/v1/agents/myagent/start/extra"},

		// Whitespace / special chars
		{"space in agent ID", "/api/v1/agents/my agent/start"},
		{"agent ID is single dot", "/api/v1/agents/./start"},
		{"agent ID starts with dash", "/api/v1/agents/-bad/start"},
		{"agent ID starts with dot", "/api/v1/agents/.hidden/start"},

		// Wrong prefix
		{"no v1 prefix", "/api/agents/myagent/start"},
		{"empty path", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := parseAgentActionPath(tc.path)
			if ok {
				t.Errorf("parseAgentActionPath(%q) = ok, want rejection", tc.path)
			}
		})
	}
}

// TestMalformedAgentPaths_HTTP exercises the full HTTP stack with malformed
// /api/v1/agents/ paths and asserts that none returns 200.
func TestMalformedAgentPaths_HTTP(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	tests := []struct {
		name       string
		path       string
		wantStatus int // 0 means "any non-200"
	}{
		// Path traversal: ServeMux cleans ".." → redirects or lands elsewhere
		{"dot-dot agent ID", "/api/v1/agents/../start", 0},
		{"encoded dot-dot", "/api/v1/agents/%2e%2e/start", http.StatusNotFound},
		{"backslash in agent ID", "/api/v1/agents/foo%5Cbar/start", http.StatusNotFound},

		// Empty / missing components
		{"trailing slash only", "/api/v1/agents/", http.StatusNotFound},
		{"agent with trailing slash", "/api/v1/agents/myagent/", http.StatusNotFound},
		{"three segments", "/api/v1/agents/myagent/start/extra", http.StatusNotFound},

		// Special agent IDs
		{"agent ID is single dot", "/api/v1/agents/./start", 0},
		{"agent ID starts with dash", "/api/v1/agents/-bad/start", http.StatusNotFound},

		// Null byte
		{"null byte encoded", "/api/v1/agents/%00agent/start", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build request manually to avoid panics from invalid percent-encoding.
			req := &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Path: tc.path, RawPath: tc.path},
				Header: make(http.Header),
				Proto:  "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if tc.wantStatus != 0 {
				if rec.Code != tc.wantStatus {
					t.Errorf("path %q: got status %d, want %d; body: %s",
						tc.path, rec.Code, tc.wantStatus, rec.Body.String())
				}
			} else if rec.Code == http.StatusOK {
				t.Errorf("path %q: got 200 OK, want non-200; body: %s",
					tc.path, rec.Body.String())
			}
		})
	}
}
