package gateway

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRemoteKeysValidationBranches(t *testing.T) {
	mux := buildRemoteFeatureMux(t)
	t.Setenv("CARRIER_REMOTE_KEY_DIR", filepath.Join(t.TempDir(), "keys"))

	t.Run("file required", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("note", "hello"); err != nil {
			t.Fatalf("write form field error: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer error: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/remote/keys", &body)
		req.Header.Set("Authorization", "Bearer test-gateway-token")
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing file, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if payload["errorCode"] != "E_USAGE" {
			t.Fatalf("unexpected error code: %#v", payload)
		}
		if message, _ := payload["errorMessage"].(string); !strings.Contains(message, "file is required") {
			t.Fatalf("unexpected error message: %#v", payload)
		}
	})

	t.Run("multipart required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/remote/keys", strings.NewReader("plain-text"))
		req.Header.Set("Authorization", "Bearer test-gateway-token")
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-multipart, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if message, _ := payload["errorMessage"].(string); !strings.Contains(message, "request must be multipart/form-data") {
			t.Fatalf("unexpected error message: %#v", payload)
		}
	})

	t.Run("invalid pem rejected", func(t *testing.T) {
		req := buildMultipartRemoteKeyRequest(t, "/api/v1/remote/keys", "bad.pem", []byte("not a pem\n"))
		req.Header.Set("Authorization", "Bearer test-gateway-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid pem, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if message, _ := payload["errorMessage"].(string); !strings.Contains(message, "PEM") {
			t.Fatalf("unexpected error message: %#v", payload)
		}
	})
}

func TestHandleRemoteKeysRequestSizeLimit(t *testing.T) {
	mux := buildRemoteFeatureMux(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	large := bytes.Repeat([]byte("a"), int(remoteKeyUploadRequestMaxBytes+10))
	part, err := writer.CreateFormFile("file", "k.pem")
	if err != nil {
		t.Fatalf("create large form file error: %v", err)
	}
	if _, err := part.Write(large); err != nil {
		t.Fatalf("write large file error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/remote/keys", &body)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too large request, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if message, _ := payload["errorMessage"].(string); !strings.Contains(message, "request body exceeds") {
		t.Fatalf("unexpected error message: %#v", payload)
	}
}
