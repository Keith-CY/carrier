package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesRootAndSPAFallback(t *testing.T) {
	h := Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	h.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("root status=%d body=%s", rootRec.Code, rootRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rootRec.Body.String()), "<html") {
		t.Fatalf("expected html response for root path")
	}
	if !strings.Contains(rootRec.Body.String(), `id="execution-list"`) {
		t.Fatalf("expected dashboard execution list container in root html")
	}
	if !strings.Contains(rootRec.Body.String(), `id="refresh-executions"`) {
		t.Fatalf("expected dashboard execution refresh button in root html")
	}

	routeReq := httptest.NewRequest(http.MethodGet, "/app/route/does-not-exist", nil)
	routeRec := httptest.NewRecorder()
	h.ServeHTTP(routeRec, routeReq)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("spa route status=%d body=%s", routeRec.Code, routeRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(routeRec.Body.String()), "<html") {
		t.Fatalf("expected html fallback for unknown SPA route")
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	assetRec := httptest.NewRecorder()
	h.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%s", assetRec.Code, assetRec.Body.String())
	}
	if strings.Contains(strings.ToLower(assetRec.Body.String()), "<html") {
		t.Fatalf("expected static asset response for /app.js")
	}
}
