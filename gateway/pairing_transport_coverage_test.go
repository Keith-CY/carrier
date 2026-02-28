package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePairingSessions_Branches(t *testing.T) {
	t.Run("session store nil", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/sessions?provider=telegram", nil)
		handlePairingSessions(rec, req, "req-nil-store", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("provider validation", func(t *testing.T) {
		sessions := NewSessionStore("", 0, nil)
		t.Cleanup(sessions.Stop)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/sessions", nil)
		handlePairingSessions(rec, req, "req-missing-provider", sessions)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing provider, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/pairing/sessions?provider=unknown", nil)
		handlePairingSessions(rec, req, "req-unsupported-provider", sessions)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for unsupported provider, got %d", rec.Code)
		}
	})

	t.Run("success filters and skips nil session records", func(t *testing.T) {
		sessions := NewSessionStore("", 0, nil)
		t.Cleanup(sessions.Stop)
		sessions.CreateSession("telegram", "100")
		sessions.CreateSession("telegram", "101")
		sessions.CreateSession("discord", "200")
		sessions.mu.Lock()
		sessions.sessions["telegram:nil"] = nil
		sessions.mu.Unlock()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/sessions?provider=TeLeGrAm", nil)
		handlePairingSessions(rec, req, "req-success", sessions)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var payload struct {
			Result   string                  `json:"result"`
			Provider string                  `json:"provider"`
			Sessions []pairingSessionSummary `json:"sessions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v; body=%s", err, rec.Body.String())
		}
		if payload.Result != "ok" {
			t.Fatalf("expected result=ok, got %+v", payload)
		}
		if payload.Provider != "telegram" {
			t.Fatalf("provider = %q, want telegram", payload.Provider)
		}
		if len(payload.Sessions) != 2 {
			t.Fatalf("expected 2 telegram sessions, got %+v", payload.Sessions)
		}
		for _, s := range payload.Sessions {
			if s.Provider != "telegram" {
				t.Fatalf("expected provider=telegram in all sessions, got %+v", payload.Sessions)
			}
		}
	})
}

func TestManagedProviderMappingAndDisplayName(t *testing.T) {
	if got := managedAgentDisplayName("picoclaw"); got != "PicoClaw" {
		t.Fatalf("managedAgentDisplayName(picoclaw) = %q", got)
	}
	if got := managedAgentDisplayName("openclaw"); got != "OpenClaw" {
		t.Fatalf("managedAgentDisplayName(openclaw) = %q", got)
	}
	if got := managedAgentDisplayName("zeroclaw"); got != "ZeroClaw" {
		t.Fatalf("managedAgentDisplayName(zeroclaw) = %q", got)
	}
	if got := managedAgentDisplayName(" worker "); got != "worker" {
		t.Fatalf("managedAgentDisplayName(worker) = %q", got)
	}

	cases := map[string]string{
		"openai-codex":      "openai",
		"openai-compatible": "openai",
		"anthropic":         "anthropic",
		"  custom  ":        "custom",
	}
	for input, want := range cases {
		if got := mapCarrierProviderToManagedProvider(input); got != want {
			t.Fatalf("mapCarrierProviderToManagedProvider(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTelegramTransportStatusAndUpdateID_Branches(t *testing.T) {
	setTelegramTransportStatus(" ", " ", "  E_FALLBACK  ", "  reason  ", "  hint  ")
	snap := snapshotTelegramTransportStatus()
	if snap.RequestedMode != telegramTransportAuto {
		t.Fatalf("requested mode = %q, want %q", snap.RequestedMode, telegramTransportAuto)
	}
	if snap.SelectedMode != "unknown" {
		t.Fatalf("selected mode = %q, want unknown", snap.SelectedMode)
	}
	if snap.ReasonCode != "E_FALLBACK" || snap.Reason != "reason" || snap.Hint != "hint" {
		t.Fatalf("unexpected trimmed status snapshot: %+v", snap)
	}

	setTelegramTransportStatus("webhook", "polling", "", "", "")
	snap = snapshotTelegramTransportStatus()
	if snap.RequestedMode != "webhook" || snap.SelectedMode != "polling" {
		t.Fatalf("unexpected explicit status snapshot: %+v", snap)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transport/telegram", nil)
	handleTelegramTransportStatus(rec, req, "req-status")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"transport"`) {
		t.Fatalf("expected transport payload, got %s", rec.Body.String())
	}

	if got := telegramUpdateID(map[string]interface{}{"update_id": float64(12)}); got != 12 {
		t.Fatalf("telegramUpdateID(float64) = %d, want 12", got)
	}
	if got := telegramUpdateID(map[string]interface{}{"update_id": int64(13)}); got != 13 {
		t.Fatalf("telegramUpdateID(int64) = %d, want 13", got)
	}
	if got := telegramUpdateID(map[string]interface{}{"update_id": "not-a-number"}); got != 0 {
		t.Fatalf("telegramUpdateID(invalid string) = %d, want 0", got)
	}
	if got := telegramUpdateID(map[string]interface{}{}); got != 0 {
		t.Fatalf("telegramUpdateID(missing) = %d, want 0", got)
	}
}
