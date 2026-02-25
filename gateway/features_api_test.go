package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func buildFeaturesMux(t *testing.T, cfg *GatewayConfig) http.Handler {
	t.Helper()
	srv := newMockDaemon(nil)
	t.Cleanup(srv.Close)

	if cfg == nil {
		cfg = &GatewayConfig{
			APIToken:            "test-gateway-token",
			MaxCommandBodyBytes: 64 * 1024,
		}
	}
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	return buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
}

func TestFeaturesEndpointMethodAndAuthGuards(t *testing.T) {
	mux := buildFeaturesMux(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized features status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/features", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method guard status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFeaturesEndpointReturnsFlags(t *testing.T) {
	mux := buildFeaturesMux(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         false,
		ProviderBindingEnabled:    true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("features status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Result   string `json:"result"`
		Features struct {
			RemoteControlPlaneEnabled bool `json:"remoteControlPlaneEnabled"`
			RemoteChatEnabled         bool `json:"remoteChatEnabled"`
			ProviderBindingEnabled    bool `json:"providerBindingEnabled"`
		} `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode features response: %v body=%s", err, rec.Body.String())
	}
	if payload.Result != "ok" {
		t.Fatalf("unexpected result: %#v", payload)
	}
	if !payload.Features.RemoteControlPlaneEnabled {
		t.Fatalf("expected remote control plane enabled=true")
	}
	if payload.Features.RemoteChatEnabled {
		t.Fatalf("expected remote chat enabled=false")
	}
	if !payload.Features.ProviderBindingEnabled {
		t.Fatalf("expected provider binding enabled=true")
	}
}

func TestFeaturesEndpointNormalizesDependentFlags(t *testing.T) {
	mux := buildFeaturesMux(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("features status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Features struct {
			RemoteControlPlaneEnabled bool `json:"remoteControlPlaneEnabled"`
			RemoteChatEnabled         bool `json:"remoteChatEnabled"`
			ProviderBindingEnabled    bool `json:"providerBindingEnabled"`
		} `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode features response: %v body=%s", err, rec.Body.String())
	}
	if payload.Features.RemoteControlPlaneEnabled {
		t.Fatalf("expected remote control disabled, got %+v", payload.Features)
	}
	if payload.Features.RemoteChatEnabled {
		t.Fatalf("expected remote chat normalized to false when remote control is disabled, got %+v", payload.Features)
	}
	if payload.Features.ProviderBindingEnabled {
		t.Fatalf("expected provider binding normalized to false when remote control is disabled, got %+v", payload.Features)
	}
}
