package gateway

import (
	"testing"
	"time"
)

func TestDownloadStore_IssueAndConsume(t *testing.T) {
	now := time.Now()
	d := NewDownloadStore("", func() time.Time { return now })

	tok := d.Issue("/tmp/artifact.zip", 5*time.Minute, true)
	if tok == nil {
		t.Fatal("Issue returned nil")
	}
	if !startsWith(tok.Token, "dl-") {
		t.Errorf("token %q should start with 'dl-'", tok.Token)
	}
	if tok.FileRef != "/tmp/artifact.zip" {
		t.Errorf("fileRef mismatch: %q", tok.FileRef)
	}

	// First consume succeeds
	got := d.Consume(tok.Token)
	if got == nil {
		t.Fatal("Consume returned nil on first call")
	}
	if got.FileRef != "/tmp/artifact.zip" {
		t.Errorf("fileRef mismatch: %q", got.FileRef)
	}

	// Second consume of single-use token fails (marked consumed)
	got2 := d.Consume(tok.Token)
	if got2 != nil {
		t.Error("second Consume of single-use token should return nil")
	}
}

func TestDownloadStore_Expiry(t *testing.T) {
	now := time.Now()
	d := NewDownloadStore("", func() time.Time { return now })

	tok := d.Issue("/tmp/f.zip", 5*time.Minute, false)

	// Advance past TTL
	now = now.Add(10 * time.Minute)

	got := d.Consume(tok.Token)
	if got != nil {
		t.Error("Consume should return nil for expired token")
	}
}

func TestDownloadStore_FinalizeConsumed(t *testing.T) {
	now := time.Now()
	d := NewDownloadStore("", func() time.Time { return now })

	tok := d.Issue("/tmp/f.zip", 5*time.Minute, true)
	d.Consume(tok.Token)          // mark as consumed
	d.FinalizeConsumed(tok.Token) // should remove from map

	if d.Size() != 0 {
		t.Errorf("expected 0 tokens after finalize, got %d", d.Size())
	}
}

func TestDownloadStore_Cleanup(t *testing.T) {
	now := time.Now()
	d := NewDownloadStore("", func() time.Time { return now })

	d.Issue("/tmp/a.zip", 1*time.Minute, false)
	d.Issue("/tmp/b.zip", 1*time.Minute, false)

	now = now.Add(2 * time.Minute)
	removed := d.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
}

func TestDownloadStore_ToDownloadURL(t *testing.T) {
	d := NewDownloadStore("", nil)
	tok := d.Issue("/tmp/my-file.zip", 5*time.Minute, true)
	url := d.ToDownloadURL(tok)
	if !startsWith(url, "/downloads/dl-") {
		t.Errorf("URL %q should start with /downloads/dl-", url)
	}
	if !containsStr2(url, "my-file.zip") {
		t.Errorf("URL %q should contain filename", url)
	}
}

func TestParseDownloadPath(t *testing.T) {
	tests := []struct {
		path      string
		wantToken string
		wantFile  string
		wantOK    bool
	}{
		{"/downloads/dl-abc/artifact.zip", "dl-abc", "artifact.zip", true},
		{"/downloads/dl-abc/my%20file.zip", "dl-abc", "my file.zip", true},
		{"/downloads/dl-abc", "", "", false},
		{"/notdownloads/dl-abc/file.zip", "", "", false},
		{"/downloads//file.zip", "", "", false},
	}
	for _, tc := range tests {
		tok, file, ok := ParseDownloadPath(tc.path)
		if ok != tc.wantOK {
			t.Errorf("ParseDownloadPath(%q) ok=%v, want %v", tc.path, ok, tc.wantOK)
			continue
		}
		if ok {
			if tok != tc.wantToken {
				t.Errorf("ParseDownloadPath(%q) token=%q, want %q", tc.path, tok, tc.wantToken)
			}
			if file != tc.wantFile {
				t.Errorf("ParseDownloadPath(%q) file=%q, want %q", tc.path, file, tc.wantFile)
			}
		}
	}
}

func TestBuildContentDisposition(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"simple.zip", `attachment; filename="simple.zip"`},
		{`has"quote.zip`, `attachment; filename="has\"quote.zip"`},
		{"日本語.zip", `attachment; filename="_____` + "." + `zip"; filename*=UTF-8''` + "%E6%97%A5%E6%9C%AC%E8%AA%9E.zip"},
	}
	for _, tc := range tests {
		got := BuildContentDisposition(tc.filename)
		// For non-ASCII, just check it contains both parts
		if !containsStr2(got, "attachment; filename=") {
			t.Errorf("BuildContentDisposition(%q) = %q, missing attachment prefix", tc.filename, got)
		}
		if tc.filename == "simple.zip" && got != tc.want {
			t.Errorf("BuildContentDisposition(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func containsStr2(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
