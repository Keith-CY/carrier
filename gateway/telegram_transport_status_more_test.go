package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetAndSnapshotTelegramTransportStatus(t *testing.T) {
	setTelegramTransportStatus("", "", "  reason-code  ", "  reason  ", "  hint  ")
	status := snapshotTelegramTransportStatus()
	if status.RequestedMode != telegramTransportAuto {
		t.Fatalf("expected requested mode fallback, got %q", status.RequestedMode)
	}
	if status.SelectedMode != "unknown" {
		t.Fatalf("expected selected mode fallback, got %q", status.SelectedMode)
	}
	if status.ReasonCode != "reason-code" || status.Reason != "reason" || status.Hint != "hint" {
		t.Fatalf("expected trimmed reason fields, got %#v", status)
	}
	if status.UpdatedAt == "" {
		t.Fatalf("expected updated timestamp")
	}

	setTelegramTransportStatus("polling", "webhook", "  code ", " ok ", " use fallback ")
	status = snapshotTelegramTransportStatus()
	if status.RequestedMode != "polling" || status.SelectedMode != "webhook" {
		t.Fatalf("expected normalized transport status, got %#v", status)
	}
	if status.ReasonCode != "code" || status.Reason != "ok" || status.Hint != "use fallback" {
		t.Fatalf("expected trimmed reason fields, got %#v", status)
	}
}

func TestHandleTelegramTransportStatusWritesJSON(t *testing.T) {
	setTelegramTransportStatus("long-polling", "webhook", "RATE_LIMIT", "retry", "reduce polling")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handleTelegramTransportStatus(rec, req, " req-xyz ")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if resp["requestId"] != " req-xyz " {
		t.Fatalf("expected requestId passthrough, got %#v", resp["requestId"])
	}
	if resp["result"] != "ok" {
		t.Fatalf("expected result ok, got %#v", resp["result"])
	}
	if _, ok := resp["transport"].(map[string]interface{}); !ok {
		t.Fatalf("expected transport object, got %#v", resp["transport"])
	}
}
