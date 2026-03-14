package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayMux_MethodAndAuthGuards(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	tests := []struct {
		name   string
		method string
		path   string
		auth   bool
		want   int
	}{
		{name: "pair init unauthorized", method: http.MethodPost, path: "/api/v1/telegram/pair/init", auth: false, want: http.StatusUnauthorized},
		{name: "pair wait unauthorized", method: http.MethodPost, path: "/api/v1/telegram/pair/wait", auth: false, want: http.StatusUnauthorized},
		{name: "transport unauthorized", method: http.MethodGet, path: "/api/v1/telegram/transport", auth: false, want: http.StatusUnauthorized},
		{name: "pairing sessions unauthorized", method: http.MethodGet, path: "/api/v1/pairing/sessions", auth: false, want: http.StatusUnauthorized},
		{name: "agents unauthorized", method: http.MethodGet, path: "/api/v1/agents", auth: false, want: http.StatusUnauthorized},
		{name: "memory unauthorized", method: http.MethodGet, path: "/api/v1/memory", auth: false, want: http.StatusUnauthorized},
		{name: "instances unauthorized", method: http.MethodGet, path: "/api/v1/instances", auth: false, want: http.StatusUnauthorized},
		{name: "orchestrator metrics unauthorized", method: http.MethodGet, path: "/api/v1/orchestrator/metrics", auth: false, want: http.StatusUnauthorized},
		{name: "execution evidence unauthorized", method: http.MethodGet, path: "/api/v1/orchestrator/executions/exec-1/evidence", auth: false, want: http.StatusUnauthorized},
		{name: "audit export unauthorized", method: http.MethodGet, path: "/api/v1/audit/export?executionId=exec-1", auth: false, want: http.StatusUnauthorized},
		{name: "triggers unauthorized", method: http.MethodGet, path: "/api/v1/triggers", auth: false, want: http.StatusUnauthorized},
		{name: "worker queue unauthorized", method: http.MethodGet, path: "/api/v1/orchestrator/workers/queue", auth: false, want: http.StatusUnauthorized},
		{name: "worker reclaim stale unauthorized", method: http.MethodPost, path: "/api/v1/orchestrator/workers/reclaim-stale", auth: false, want: http.StatusUnauthorized},
		{name: "pair init method guard", method: http.MethodGet, path: "/api/v1/telegram/pair/init", auth: true, want: http.StatusMethodNotAllowed},
		{name: "pair wait method guard", method: http.MethodGet, path: "/api/v1/telegram/pair/wait", auth: true, want: http.StatusMethodNotAllowed},
		{name: "transport method guard", method: http.MethodPost, path: "/api/v1/telegram/transport", auth: true, want: http.StatusMethodNotAllowed},
		{name: "pairing sessions method guard", method: http.MethodPost, path: "/api/v1/pairing/sessions", auth: true, want: http.StatusMethodNotAllowed},
		{name: "memory search method guard", method: http.MethodGet, path: "/api/v1/memory/search", auth: true, want: http.StatusMethodNotAllowed},
		{name: "trigger webhook disabled when feature off", method: http.MethodGet, path: "/api/v1/triggers/webhook/test-trigger", auth: false, want: http.StatusNotFound},
		{name: "orchestrator metrics disabled when feature off", method: http.MethodPost, path: "/api/v1/orchestrator/metrics", auth: true, want: http.StatusNotFound},
		{name: "execution evidence disabled when feature off", method: http.MethodGet, path: "/api/v1/orchestrator/executions/exec-1/evidence", auth: true, want: http.StatusNotFound},
		{name: "audit export disabled when feature off", method: http.MethodGet, path: "/api/v1/audit/export?executionId=exec-1", auth: true, want: http.StatusNotFound},
		{name: "worker queue disabled when feature off", method: http.MethodPost, path: "/api/v1/orchestrator/workers/queue", auth: true, want: http.StatusNotFound},
		{name: "worker reclaim stale disabled when feature off", method: http.MethodGet, path: "/api/v1/orchestrator/workers/reclaim-stale", auth: true, want: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.auth {
				req.Header.Set("Authorization", "Bearer test-gateway-token")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("expected %d, got %d (%s)", tc.want, w.Code, w.Body.String())
			}
		})
	}
}

func TestRequestIDFromCtx_MissingReturnsEmpty(t *testing.T) {
	if got := requestIDFromCtx(context.Background()); got != "" {
		t.Fatalf("expected empty request id, got %q", got)
	}
}

func TestGatewayErrBody_DefaultMessageWhenTrimmedEmpty(t *testing.T) {
	body := gatewayErrBody("E_TEST", "   ")
	if body["result"] != "error" {
		t.Fatalf("expected result=error, got %#v", body)
	}
	if body["errorCode"] != "E_TEST" {
		t.Fatalf("expected errorCode=E_TEST, got %#v", body)
	}
	if body["message"] != "request failed" {
		t.Fatalf("expected fallback message, got %#v", body)
	}
}
