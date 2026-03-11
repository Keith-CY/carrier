package webui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
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
	rootBody := rootRec.Body.String()
	if !strings.Contains(strings.ToLower(rootBody), "<html") {
		t.Fatalf("expected html response for root path")
	}
	if !strings.Contains(rootBody, `id="root"`) {
		t.Fatalf("expected react root container in root html")
	}
	if !strings.Contains(rootBody, `/assets/index-`) {
		t.Fatalf("expected vite asset references in root html")
	}
	if strings.Contains(rootBody, `./app.js`) {
		t.Fatalf("expected legacy shell bootstrap asset to be removed from root html")
	}
	if strings.Contains(rootBody, `remote-control-islands.js`) {
		t.Fatalf("expected legacy island asset to be removed from root html")
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

	match := regexp.MustCompile(`/assets/[^"]+\.js`).FindString(rootBody)
	if match == "" {
		t.Fatalf("expected javascript asset reference in root html")
	}
	assetReq := httptest.NewRequest(http.MethodGet, match, nil)
	assetRec := httptest.NewRecorder()
	h.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%s", assetRec.Code, assetRec.Body.String())
	}
	if strings.Contains(strings.ToLower(assetRec.Body.String()), "<html") {
		t.Fatalf("expected static asset response for /app.js")
	}
}
