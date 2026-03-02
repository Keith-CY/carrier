package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildMultipartRemoteKeyRequest(t *testing.T, url, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if content != nil {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create multipart form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write multipart file content: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, &body)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestRemoteKeyHelpersAndSaveUploadedKey(t *testing.T) {
	keyDir := t.TempDir()
	t.Setenv("CARRIER_REMOTE_KEY_DIR", keyDir)

	dir, err := remoteKeyDirPath()
	if err != nil {
		t.Fatalf("remoteKeyDirPath error: %v", err)
	}
	if filepath.Clean(dir) != filepath.Clean(keyDir) {
		t.Fatalf("unexpected key dir=%q want=%q", dir, keyDir)
	}
	if _, err := resolveRemoteKeyPath("bad"); err == nil {
		t.Fatalf("expected invalid key ref error")
	}

	rawPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3, 4}})
	if err := validatePEMPrivateKey(rawPEM); err != nil {
		t.Fatalf("validatePEMPrivateKey valid pem error: %v", err)
	}
	if err := validatePEMPrivateKey([]byte("not a pem key")); err == nil {
		t.Fatalf("expected invalid PEM validation error")
	}

	fp1 := pemFingerprint(rawPEM)
	fp2 := pemFingerprint(rawPEM)
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("expected deterministic fingerprint, fp1=%q fp2=%q", fp1, fp2)
	}

	if _, err := saveUploadedRemoteKey("x.pem", nil); err == nil {
		t.Fatalf("expected empty PEM upload error")
	}
	tooLarge := bytes.Repeat([]byte("a"), remoteKeyUploadMaxBytes+1)
	if _, err := saveUploadedRemoteKey("x.pem", tooLarge); err == nil {
		t.Fatalf("expected oversize PEM upload error")
	}

	uploaded, err := saveUploadedRemoteKey("my-key.pem", rawPEM)
	if err != nil {
		t.Fatalf("saveUploadedRemoteKey error: %v", err)
	}
	if uploaded.KeyRef == "" || uploaded.Fingerprint == "" || uploaded.SizeBytes == 0 {
		t.Fatalf("unexpected uploaded key payload: %+v", uploaded)
	}
	path, err := resolveRemoteKeyPath(uploaded.KeyRef)
	if err != nil {
		t.Fatalf("resolveRemoteKeyPath uploaded ref error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected saved key file at %s: %v", path, err)
	}
}

func TestHandleRemoteKeysEndpoint(t *testing.T) {
	t.Run("feature disabled", func(t *testing.T) {
		mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
			APIToken:                  "test-gateway-token",
			MaxCommandBodyBytes:       64 * 1024,
			RemoteControlPlaneEnabled: false,
			RemoteChatEnabled:         true,
			ProviderBindingEnabled:    true,
		}, nil)
		req := buildMultipartRemoteKeyRequest(t, "/api/v1/remote/keys", "k.pem", []byte("-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		mux := buildRemoteFeatureMux(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/remote/keys", nil)
		req.Header.Set("Authorization", "Bearer test-gateway-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("happy path and validation", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_KEY_DIR", t.TempDir())
		mux := buildRemoteFeatureMux(t)
		rawPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{5, 6, 7}})
		req := buildMultipartRemoteKeyRequest(t, "/api/v1/remote/keys", "uploaded.pem", rawPEM)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["result"] != "ok" {
			t.Fatalf("unexpected payload: %v", payload)
		}

		plainReq := httptest.NewRequest(http.MethodPost, "/api/v1/remote/keys", strings.NewReader(`{"x":1}`))
		plainReq.Header.Set("Authorization", "Bearer test-gateway-token")
		plainReq.Header.Set("Content-Type", "application/json")
		plainRec := httptest.NewRecorder()
		mux.ServeHTTP(plainRec, plainReq)
		if plainRec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", plainRec.Code, plainRec.Body.String())
		}
	})
}

func TestRemoteAlertPayloadAndPostHelpers(t *testing.T) {
	payload, err := marshalRemoteAlertPayload(
		remoteMetricsSnapshot{
			Alerts: remoteAlertSummary{Active: true, Level: "canary", Count: 1},
			Rollout: remoteRolloutStatus{
				State:      "canary",
				CanPromote: false,
				Reasons:    []string{"operation success rate below 98%"},
			},
		},
		remoteAlertDigest{Active: true, Level: "canary", Count: 1, Reasons: "operation success rate below 98%"},
		remoteAlertDigest{Active: false, Level: "none", Count: 0, Reasons: ""},
		true,
		"state-change",
		time.Unix(1700000000, 0),
	)
	if err != nil {
		t.Fatalf("marshalRemoteAlertPayload error: %v", err)
	}
	if !strings.Contains(string(payload), `"trigger":"state-change"`) {
		t.Fatalf("missing trigger in payload: %s", string(payload))
	}
	if !strings.Contains(string(payload), `"previous"`) {
		t.Fatalf("missing previous section in payload: %s", string(payload))
	}

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()

	if err := defaultRemoteAlertPost(context.Background(), nil, okServer.URL, payload); err != nil {
		t.Fatalf("defaultRemoteAlertPost success error: %v", err)
	}

	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "bad gateway")
	}))
	defer errServer.Close()

	if err := defaultRemoteAlertPost(context.Background(), nil, errServer.URL, payload); err == nil {
		t.Fatalf("expected non-2xx webhook error")
	}
}

func TestRemoteAlertWatchdogTickStateTransitions(t *testing.T) {
	resetRemoteMetricsForTests()
	t.Cleanup(resetRemoteMetricsForTests)

	base := time.Unix(1700000000, 0)
	now := base
	var sentPayloads []string
	watchdog := &remoteAlertWatchdog{
		webhookURL: "http://example.test/webhook",
		interval:   time.Second,
		cooldown:   5 * time.Minute,
		client:     &http.Client{Timeout: time.Second},
		now: func() time.Time {
			return now
		},
		post: func(_ context.Context, _ *http.Client, _ string, payload []byte) error {
			sentPayloads = append(sentPayloads, string(payload))
			return nil
		},
	}

	for i := 0; i < 5; i++ {
		success := i != 0
		remoteMetrics.recordOperation(remoteOpInstancesInstall, success, 1200*time.Millisecond)
	}
	watchdog.tick(context.Background())
	if len(sentPayloads) != 1 {
		t.Fatalf("expected first active alert payload, got %d", len(sentPayloads))
	}
	if !strings.Contains(sentPayloads[0], `"state":"active"`) {
		t.Fatalf("expected active state payload, got %s", sentPayloads[0])
	}

	now = now.Add(2 * time.Minute)
	watchdog.tick(context.Background())
	if len(sentPayloads) != 1 {
		t.Fatalf("expected no resend before state/cooldown change, got %d", len(sentPayloads))
	}

	resetRemoteMetricsForTests()
	now = now.Add(10 * time.Second)
	watchdog.tick(context.Background())
	if len(sentPayloads) != 2 {
		t.Fatalf("expected resolved payload after metrics reset, got %d", len(sentPayloads))
	}
	if !strings.Contains(sentPayloads[1], `"state":"resolved"`) {
		t.Fatalf("expected resolved state payload, got %s", sentPayloads[1])
	}
}

func TestStartRemoteAlertWatchdogAndBindingSplitHelper(t *testing.T) {
	startRemoteAlertWatchdog(context.Background(), nil)
	startRemoteAlertWatchdog(context.Background(), &GatewayConfig{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	startRemoteAlertWatchdog(ctx, &GatewayConfig{
		RemoteAlertWebhookURL: server.URL,
		RemoteAlertInterval:   20 * time.Millisecond,
		RemoteAlertCooldown:   0,
	})
	time.Sleep(30 * time.Millisecond)
	cancel()

	if h, a := splitInstanceBindingTarget("host-1:main"); h != "host-1" || a != "main" {
		t.Fatalf("unexpected split for colon target: host=%q agent=%q", h, a)
	}
	if h, a := splitInstanceBindingTarget("host-2/main"); h != "host-2" || a != "main" {
		t.Fatalf("unexpected split for slash target: host=%q agent=%q", h, a)
	}
	if h, a := splitInstanceBindingTarget("invalid"); h != "" || a != "" {
		t.Fatalf("unexpected split for invalid target: host=%q agent=%q", h, a)
	}
}

func TestHandleRemoteHostMemoryAndInstallViaGUIOnlyResp(t *testing.T) {
	t.Run("memory method not allowed", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			return remoteExecResult{ExitCode: 0, Stdout: ""}
		})
		mux := buildRemoteFeatureMux(t)
		hostID := createRemoteHostForTests(t, mux)
		rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/memory", "{}")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("memory success", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, "find \"$base\" -type f") {
				return remoteExecResult{ExitCode: 0, Stdout: "memory/a.txt\t12\t1700000000\n"}
			}
			return remoteExecResult{ExitCode: 0, Stdout: ""}
		})
		mux := buildRemoteFeatureMux(t)
		hostID := createRemoteHostForTests(t, mux)
		rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts/"+hostID+"/memory?agentId=main", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		body := decodeJSONMap(t, rec)
		memoryEntries, _ := body["memory"].([]interface{})
		if len(memoryEntries) != 1 {
			t.Fatalf("expected one memory entry, payload=%v", body)
		}
	})

	t.Run("install gui-only response", func(t *testing.T) {
		resp := installViaGUIOnlyResp("req-1")
		if resp.ErrorCode != "E_INSTALL_GUI_ONLY" {
			t.Fatalf("unexpected error code: %+v", resp)
		}
		if !strings.Contains(strings.ToLower(resp.Message), "carrier") {
			t.Fatalf("expected carrier hint in message: %+v", resp)
		}
	})
}
