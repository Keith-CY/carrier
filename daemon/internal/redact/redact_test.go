package redact

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRedactEnvironMasksAllValues(t *testing.T) {
	env := []string{
		"OPENAI_API_KEY=sk-live-abc",
		"PATH=/usr/bin",
		"EMPTY=",
		" NO_VALUE ",
	}

	got := RedactEnviron(env)
	if got["OPENAI_API_KEY"] != RedactedValue {
		t.Fatalf("expected OPENAI_API_KEY to be redacted, got %q", got["OPENAI_API_KEY"])
	}
	if got["PATH"] != "/usr/bin" {
		t.Fatalf("expected PATH to be preserved, got %q", got["PATH"])
	}
	if got["EMPTY"] != "" {
		t.Fatalf("expected EMPTY to be empty string, got %q", got["EMPTY"])
	}
	if got["NO_VALUE"] != "" {
		t.Fatalf("expected NO_VALUE to be empty string, got %q", got["NO_VALUE"])
	}
}

func TestRedactEnvironAppliesSecondaryTextFilter(t *testing.T) {
	env := []string{
		"DATABASE_URL=postgres://user:my_secret_password@host:5432/db",
	}
	got := RedactEnviron(env)
	if strings.Contains(got["DATABASE_URL"], "my_secret_password") {
		t.Fatalf("expected DATABASE_URL value to have secrets redacted via text filter, got %q", got["DATABASE_URL"])
	}
}

func TestIsSensitiveKeyName(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{key: "OPENAI_API_KEY", want: true},
		{key: "db_password", want: true},
		{key: "authToken", want: true},
		{key: "service_credential_file", want: true},
		{key: "PATH", want: false},
	}
	for _, tc := range tests {
		got := IsSensitiveKeyName(tc.key)
		if got != tc.want {
			t.Fatalf("IsSensitiveKeyName(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestRedactTextScrubsSensitiveAssignments(t *testing.T) {
	input := strings.Join([]string{
		`OPENAI_API_KEY=sk-live-abc`,
		`token: abc123`,
		`PASSWORD = hunter2`,
		`PATH=/usr/bin`,
	}, "\n")

	got := RedactText(input)
	if strings.Contains(got, "sk-live-abc") || strings.Contains(got, "abc123") || strings.Contains(got, "hunter2") {
		t.Fatalf("expected sensitive values to be redacted, got %q", got)
	}
	if !strings.Contains(got, "PATH=/usr/bin") {
		t.Fatalf("expected non-sensitive key/value to remain, got %q", got)
	}
}

func TestMetadataJSONIncludesDefault24HourExpiryAndChecksum(t *testing.T) {
	created := time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)
	artifacts := map[string][]byte{
		"logs.txt":      []byte("hello"),
		"manifest.json": []byte(`{"id":"openclaw"}`),
	}

	payload, err := MetadataJSON(created, 0, artifacts)
	if err != nil {
		t.Fatalf("MetadataJSON: %v", err)
	}

	var meta ArtifactMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}

	if !meta.CreatedAt.Equal(created) {
		t.Fatalf("created_at mismatch: got %s want %s", meta.CreatedAt, created)
	}
	wantExpiry := created.Add(24 * time.Hour)
	if !meta.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expires_at mismatch: got %s want %s", meta.ExpiresAt, wantExpiry)
	}
	if len(meta.SHA256) != 64 {
		t.Fatalf("expected sha256 hex length 64, got %d", len(meta.SHA256))
	}
}
