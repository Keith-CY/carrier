package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFeatureFlagsNormalization(t *testing.T) {
	cfg := effectiveGatewayFeatureFlags(&GatewayConfig{RemoteControlPlaneEnabled: true, RemoteChatEnabled: true, ProviderBindingEnabled: true})
	if !cfg.RemoteControlPlaneEnabled {
		t.Fatalf("expected remote control plane enabled")
	}
	if !cfg.RemoteChatEnabled {
		t.Fatalf("expected remote chat enabled when dependency is enabled")
	}
	if !cfg.ProviderBindingEnabled {
		t.Fatalf("expected provider binding enabled when dependency is enabled")
	}

	cfg = effectiveGatewayFeatureFlags(&GatewayConfig{RemoteControlPlaneEnabled: false, RemoteChatEnabled: true, ProviderBindingEnabled: true})
	if cfg.RemoteControlPlaneEnabled {
		t.Fatalf("expected remote control plane disabled")
	}
	if cfg.RemoteChatEnabled || cfg.ProviderBindingEnabled {
		t.Fatalf("expected dependent flags disabled when remote control plane disabled: %#v", cfg)
	}
}

func TestFeatureFlagsNormalizationResetsWhenDependencyDisabled(t *testing.T) {
	cfg := &GatewayConfig{RemoteControlPlaneEnabled: false, RemoteChatEnabled: true, ProviderBindingEnabled: true}
	normalizeGatewayConfigFeatureFlags(cfg)
	if cfg.RemoteChatEnabled || cfg.ProviderBindingEnabled {
		t.Fatalf("expected dependent flags reset: %#v", cfg)
	}

	normalizeGatewayConfigFeatureFlags(nil)
}

func TestWebUIHandlerFactoryLifecycle(t *testing.T) {
	SetWebUIHandlerFactory(nil)
	t.Cleanup(func() {
		SetWebUIHandlerFactory(nil)
	})

	handler := webUIHandler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected default webui handler not found, got %d", recorder.Code)
	}

	called := false
	SetWebUIHandlerFactory(func() http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusCreated)
		})
	})
	handler = webUIHandler()
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatalf("expected custom webui handler factory to be invoked")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected custom handler status, got %d", recorder.Code)
	}

	SetWebUIHandlerFactory(nil)
	handler = webUIHandler()
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected fallback not found after nil factory, got %d", recorder.Code)
	}
}
