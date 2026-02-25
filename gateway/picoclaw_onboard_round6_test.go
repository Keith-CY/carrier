package gateway

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupIfExists_StatErrorBranch(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	// os.Stat on blocker/child returns a non-IsNotExist error (not a directory).
	err := backupIfExists(filepath.Join(blocker, "child"))
	if err == nil {
		t.Fatal("expected stat error")
	}
}

func TestActorChatID_AdditionalInvalidBranches(t *testing.T) {
	if got := actorChatID("telegram"); got != "" {
		t.Fatalf("expected empty chat id without colon, got %q", got)
	}
	if got := actorChatID("telegram:   "); got != "" {
		t.Fatalf("expected empty chat id for blank suffix, got %q", got)
	}
}

func TestPickProviderToken_Branches(t *testing.T) {
	if got := pickProviderToken(nil, map[string]string{"OPENAI_API_KEY": "x"}); got != "" {
		t.Fatalf("expected empty token for nil provider, got %q", got)
	}
	if got := pickProviderToken(&LLMProvider{ID: "openai"}, nil); got != "" {
		t.Fatalf("expected empty token for nil env map, got %q", got)
	}

	codex := &LLMProvider{ID: "openai-codex", EnvVar: "OPENAI_CODEX_TOKEN"}
	if got := pickProviderToken(codex, map[string]string{"OPENAI_API_KEY": "from-alias"}); got != "from-alias" {
		t.Fatalf("expected OPENAI_API_KEY alias fallback, got %q", got)
	}
	custom := &LLMProvider{ID: "custom", EnvVar: "CUSTOM_KEY"}
	if got := pickProviderToken(custom, map[string]string{"CUSTOM_KEY": "custom-token"}); got != "custom-token" {
		t.Fatalf("expected provider env token, got %q", got)
	}
	if got := pickProviderToken(&LLMProvider{ID: "custom", EnvVar: ""}, map[string]string{"ANY": "x"}); got != "" {
		t.Fatalf("expected empty token for empty env var, got %q", got)
	}
}

func TestSavePicoclawAuthCredential_Branches(t *testing.T) {
	if err := savePicoclawAuthCredential(t.TempDir(), "openai", "", "acct"); err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("expected empty access token error, got %v", err)
	}

	t.Run("mkdir error", func(t *testing.T) {
		tmp := t.TempDir()
		homeAsFile := filepath.Join(tmp, "home-file")
		if err := os.WriteFile(homeAsFile, []byte("x"), 0o600); err != nil {
			t.Fatalf("write home file: %v", err)
		}
		err := savePicoclawAuthCredential(homeAsFile, "openai", "tok", "acct")
		if err == nil || !strings.Contains(err.Error(), "create auth dir") {
			t.Fatalf("expected create auth dir error, got %v", err)
		}
	})

	t.Run("parse existing store error", func(t *testing.T) {
		tmp := t.TempDir()
		authPath := filepath.Join(tmp, ".picoclaw", "auth.json")
		if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
			t.Fatalf("mkdir auth dir: %v", err)
		}
		if err := os.WriteFile(authPath, []byte("{not-json"), 0o600); err != nil {
			t.Fatalf("write malformed auth store: %v", err)
		}
		err := savePicoclawAuthCredential(tmp, "openai", "tok", "acct")
		if err == nil || !strings.Contains(err.Error(), "parse existing auth store") {
			t.Fatalf("expected parse existing auth store error, got %v", err)
		}
	})

	t.Run("existing credentials nil map", func(t *testing.T) {
		tmp := t.TempDir()
		authPath := filepath.Join(tmp, ".picoclaw", "auth.json")
		if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
			t.Fatalf("mkdir auth dir: %v", err)
		}
		if err := os.WriteFile(authPath, []byte(`{"credentials":null}`), 0o600); err != nil {
			t.Fatalf("write auth store with nil credentials: %v", err)
		}
		if err := savePicoclawAuthCredential(tmp, "openai", "tok-new", "acct-new"); err != nil {
			t.Fatalf("savePicoclawAuthCredential: %v", err)
		}
		raw, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatalf("read auth store: %v", err)
		}
		if !strings.Contains(string(raw), "tok-new") || !strings.Contains(string(raw), "acct-new") {
			t.Fatalf("expected updated credential in auth store, got %s", raw)
		}
	})
}

func TestExtractOpenAIAccountID_NestedAuthClaimAndInvalidToken(t *testing.T) {
	nestedToken := "hdr." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"acct-nested"}}`)) + ".sig"
	if got := extractOpenAIAccountID(nestedToken); got != "acct-nested" {
		t.Fatalf("expected nested account id acct-nested, got %q", got)
	}
	if got := extractOpenAIAccountID("not-a-jwt"); got != "" {
		t.Fatalf("expected empty account id for invalid token, got %q", got)
	}
}

func TestParseJWTClaims_ErrorAndFallbackBranches(t *testing.T) {
	if _, err := parseJWTClaims("not-a-jwt"); err == nil || !strings.Contains(err.Error(), "not a JWT") {
		t.Fatalf("expected not-a-JWT error, got %v", err)
	}

	if _, err := parseJWTClaims("hdr.%%% .sig"); err == nil || !strings.Contains(err.Error(), "decode jwt payload") {
		t.Fatalf("expected decode jwt payload error, got %v", err)
	}

	invalidJSONPayload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	if _, err := parseJWTClaims(fmt.Sprintf("hdr.%s.sig", invalidJSONPayload)); err == nil || !strings.Contains(err.Error(), "decode jwt claims") {
		t.Fatalf("expected decode jwt claims error, got %v", err)
	}

	// Force RawURLEncoding failure (padding), then URLEncoding fallback success.
	paddedPayload := base64.URLEncoding.EncodeToString([]byte(`{"ok":true}`))
	claims, err := parseJWTClaims(fmt.Sprintf("hdr.%s.sig", paddedPayload))
	if err != nil {
		t.Fatalf("expected URLEncoding fallback success, got %v", err)
	}
	if ok, _ := claims["ok"].(bool); !ok {
		t.Fatalf("expected parsed claim ok=true, got %#v", claims)
	}
}
