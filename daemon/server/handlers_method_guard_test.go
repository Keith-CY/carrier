package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlersRejectUnexpectedMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		run    func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "install_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleInstall(nil, "openclaw", w, r)
			},
		},
		{
			name:   "uninstall_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleUninstall(nil, "openclaw", w, r)
			},
		},
		{
			name:   "start_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleStart(nil, "openclaw", w, r)
			},
		},
		{
			name:   "stop_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleStop(nil, "openclaw", w, r)
			},
		},
		{
			name:   "status_requires_get",
			method: http.MethodPost,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleStatus(nil, "openclaw", w, r)
			},
		},
		{
			name:   "logs_requires_get",
			method: http.MethodPost,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleLogs(nil, "openclaw", w, r)
			},
		},
		{
			name:   "upgrade_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleUpgrade(nil, "openclaw", w, r)
			},
		},
		{
			name:   "diagnose_requires_post",
			method: http.MethodGet,
			run: func(w http.ResponseWriter, r *http.Request) {
				handleDiagnose(nil, "openclaw", w, r)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/api/v1/agents/openclaw", nil)

			tc.run(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if !strings.Contains(rec.Body.String(), "method not allowed") {
				t.Fatalf("unexpected body: %s", rec.Body.String())
			}
		})
	}
}
