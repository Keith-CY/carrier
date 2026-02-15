package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/api"
	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/health"
	"carrier/daemon/internal/lifecycle"
)

func TestStopAllAgents_NoRunning(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := stopAllAgents(svc); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestShutdownAgents_Timeout(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	err := shutdownAgents(svc, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestShutdownAgents_VeryShortTimeout(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	err := shutdownAgents(svc, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestBuildHTTPHandlerMountsHealthAndAPI(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	healthServer := health.NewServer(svc)
	healthServer.SetReady(true)
	pairStore := api.NewPairingCodeStore(nil)
	handler := buildHTTPHandler(svc, healthServer, pairStore)

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", healthRec.Code)
	}

	agentsReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	agentsRec := httptest.NewRecorder()
	handler.ServeHTTP(agentsRec, agentsReq)
	if agentsRec.Code != http.StatusOK {
		t.Fatalf("/api/v1/agents status = %d, want 200 body=%s", agentsRec.Code, agentsRec.Body.String())
	}
}

func TestBearerAuthMiddleware(t *testing.T) {
	const token = "secret-token"

	tests := []struct {
		name           string
		path           string
		authorization  string
		wantStatus     int
		wantBody       string
		wantNextCalled bool
		wantJSON       bool
	}{
		{
			name:           "api missing authorization is unauthorized",
			path:           "/api/v1/agents",
			wantStatus:     http.StatusUnauthorized,
			wantBody:       `{"error":"unauthorized"}`,
			wantNextCalled: false,
			wantJSON:       true,
		},
		{
			name:           "api wrong bearer token is unauthorized",
			path:           "/api/v1/agents",
			authorization:  "Bearer wrong-token",
			wantStatus:     http.StatusUnauthorized,
			wantBody:       `{"error":"unauthorized"}`,
			wantNextCalled: false,
			wantJSON:       true,
		},
		{
			name:           "api correct bearer token passes through",
			path:           "/api/v1/agents",
			authorization:  "Bearer " + token,
			wantStatus:     http.StatusAccepted,
			wantBody:       "ok",
			wantNextCalled: true,
		},
		{
			name:           "non api healthz bypasses auth",
			path:           "/healthz",
			wantStatus:     http.StatusAccepted,
			wantBody:       "ok",
			wantNextCalled: true,
		},
		{
			name:           "non api readyz bypasses auth",
			path:           "/readyz",
			wantStatus:     http.StatusAccepted,
			wantBody:       "ok",
			wantNextCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("ok"))
			})

			handler := bearerAuthMiddleware(token, next)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %q, want %q", got, tc.wantBody)
			}
			if nextCalled != tc.wantNextCalled {
				t.Fatalf("nextCalled = %v, want %v", nextCalled, tc.wantNextCalled)
			}
			if tc.wantJSON {
				ct := rec.Header().Get("Content-Type")
				if !strings.HasPrefix(ct, "application/json") {
					t.Fatalf("content-type = %q, want application/json", ct)
				}
			}
		})
	}
}
