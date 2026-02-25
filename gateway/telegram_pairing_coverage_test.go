package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTelegramGetUpdatesServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	if status == 0 {
		status = http.StatusOK
	}
	return newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/getUpdates") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false,"description":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestHandleTelegramPairInit_Branches(t *testing.T) {
	t.Run("store nil", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/init", strings.NewReader(`{"token":"t"}`))
		handleTelegramPairInit(rec, req, "req-nil-store", &GatewayConfig{}, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("invalid json and missing token", func(t *testing.T) {
		store := newTelegramPairStore()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/init", strings.NewReader(`{"token"`))
		handleTelegramPairInit(rec, req, "req-invalid-json", &GatewayConfig{}, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/init", strings.NewReader(`{"token":"   "}`))
		handleTelegramPairInit(rec, req, "req-missing-token", &GatewayConfig{}, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing token, got %d", rec.Code)
		}
	})

	t.Run("token validation error", func(t *testing.T) {
		srv := newTelegramGetUpdatesServer(t, http.StatusInternalServerError, `{"ok":false,"description":"bad token"}`)
		store := newTelegramPairStore()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/init", strings.NewReader(`{"token":"bad-token"}`))
		handleTelegramPairInit(rec, req, "req-token-invalid", &GatewayConfig{TelegramAPIBaseURL: srv.URL}, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_TELEGRAM_AUTH"`) {
			t.Fatalf("expected E_TELEGRAM_AUTH, got %s", rec.Body.String())
		}
	})

	t.Run("success computes next offset and creates session", func(t *testing.T) {
		srv := newTelegramGetUpdatesServer(t, http.StatusOK, `{"ok":true,"result":[{"update_id":1},{"update_id":5}]}`)
		store := newTelegramPairStore()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/init", strings.NewReader(`{"token":"good-token"}`))
		handleTelegramPairInit(rec, req, "req-init-ok", &GatewayConfig{TelegramAPIBaseURL: srv.URL}, store)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		sessionID, _ := payload["sessionId"].(string)
		pairCode, _ := payload["pairCode"].(string)
		if strings.TrimSpace(sessionID) == "" || !strings.HasPrefix(pairCode, "tg-") {
			t.Fatalf("unexpected init payload: %+v", payload)
		}
		sess, ok := store.get(sessionID)
		if !ok {
			t.Fatalf("expected created session %q in store", sessionID)
		}
		if sess.NextOffset != 6 {
			t.Fatalf("expected nextOffset=6, got %d", sess.NextOffset)
		}
	})
}

func TestHandleTelegramPairWait_Branches(t *testing.T) {
	t.Run("store nil / invalid body / missing sessionId / not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"x"}`))
		handleTelegramPairWait(rec, req, "req-wait-nil-store", &GatewayConfig{}, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}

		store := newTelegramPairStore()
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId"`))
		handleTelegramPairWait(rec, req, "req-wait-invalid-json", &GatewayConfig{}, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"   "}`))
		handleTelegramPairWait(rec, req, "req-wait-missing-session", &GatewayConfig{}, store)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing sessionId, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"missing"}`))
		handleTelegramPairWait(rec, req, "req-wait-not-found", &GatewayConfig{}, store)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing session, got %d", rec.Code)
		}
	})

	t.Run("already paired session short-circuits", func(t *testing.T) {
		store := newTelegramPairStore()
		sess, err := store.create("good-token", 0)
		if err != nil {
			t.Fatalf("store.create: %v", err)
		}
		sess.PairedChatID = "418258935"
		store.save(sess)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"`+sess.ID+`"}`))
		handleTelegramPairWait(rec, req, "req-wait-already-paired", &GatewayConfig{}, store)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"paired":true`) {
			t.Fatalf("expected paired=true, got %s", rec.Body.String())
		}
	})

	t.Run("telegram api error", func(t *testing.T) {
		srv := newTelegramGetUpdatesServer(t, http.StatusInternalServerError, `{"ok":false,"description":"boom"}`)
		store := newTelegramPairStore()
		sess, err := store.create("good-token", 0)
		if err != nil {
			t.Fatalf("store.create: %v", err)
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"`+sess.ID+`"}`))
		handleTelegramPairWait(rec, req, "req-wait-api-error", &GatewayConfig{TelegramAPIBaseURL: srv.URL}, store)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_TELEGRAM_API"`) {
			t.Fatalf("expected E_TELEGRAM_API, got %s", rec.Body.String())
		}
	})

	t.Run("timeout branch", func(t *testing.T) {
		oldWait := telegramPairWaitTimeout
		oldPoll := telegramPairPollTimeoutS
		telegramPairWaitTimeout = 80 * time.Millisecond
		telegramPairPollTimeoutS = 1
		t.Cleanup(func() {
			telegramPairWaitTimeout = oldWait
			telegramPairPollTimeoutS = oldPoll
		})

		srv := newTelegramGetUpdatesServer(t, http.StatusOK, `{"ok":true,"result":[]}`)
		store := newTelegramPairStore()
		sess, err := store.create("good-token", 7)
		if err != nil {
			t.Fatalf("store.create: %v", err)
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"`+sess.ID+`"}`))
		handleTelegramPairWait(rec, req, "req-wait-timeout", &GatewayConfig{TelegramAPIBaseURL: srv.URL}, store)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"paired":false`) {
			t.Fatalf("expected paired=false timeout response, got %s", rec.Body.String())
		}
	})

	t.Run("pair command detected", func(t *testing.T) {
		oldWait := telegramPairWaitTimeout
		oldPoll := telegramPairPollTimeoutS
		telegramPairWaitTimeout = 500 * time.Millisecond
		telegramPairPollTimeoutS = 1
		t.Cleanup(func() {
			telegramPairWaitTimeout = oldWait
			telegramPairPollTimeoutS = oldPoll
		})

		srv := newTelegramGetUpdatesServer(t, http.StatusOK, `{"ok":true,"result":[{"update_id":10,"message":{"chat":{"id":418258935},"text":"/pair tg-abc123"}}]}`)
		store := newTelegramPairStore()
		sess, err := store.create("good-token", 0)
		if err != nil {
			t.Fatalf("store.create: %v", err)
		}
		sess.PairCode = "tg-abc123"
		store.save(sess)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/pair/wait", strings.NewReader(`{"sessionId":"`+sess.ID+`"}`))
		handleTelegramPairWait(rec, req, "req-wait-paired", &GatewayConfig{TelegramAPIBaseURL: srv.URL}, store)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"paired":true`) {
			t.Fatalf("expected paired=true response, got %s", rec.Body.String())
		}
	})
}

func TestTelegramPairStoreAndHelpers_Branches(t *testing.T) {
	t.Run("save nil-safe and prune expired", func(t *testing.T) {
		var nilStore *telegramPairStore
		nilStore.save(nil)

		store := newTelegramPairStore()
		sess, err := store.create("token", 0)
		if err != nil {
			t.Fatalf("store.create: %v", err)
		}
		store.mu.Lock()
		store.sessions[sess.ID].ExpiresAt = time.Now().UTC().Add(-1 * time.Second)
		store.pruneExpiredLocked(time.Now().UTC())
		_, ok := store.sessions[sess.ID]
		store.mu.Unlock()
		if ok {
			t.Fatal("expected expired session to be pruned")
		}
	})

	t.Run("randomHex input guard", func(t *testing.T) {
		if _, err := randomHex(0); err == nil {
			t.Fatal("expected error for non-positive numBytes")
		}
	})
}
