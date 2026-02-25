package redact

import "testing"

func TestRedactEnvironSkipsEmptyKeyEntries(t *testing.T) {
	env := []string{
		"=no-key",
		"   =trimmed-empty-key",
		"NO_SEPARATOR",
	}

	got := RedactEnviron(env)
	if _, exists := got[""]; exists {
		t.Fatal("expected empty key to be ignored")
	}
	if got["NO_SEPARATOR"] != "" {
		t.Fatalf("expected NO_SEPARATOR to be empty value, got %q", got["NO_SEPARATOR"])
	}
}
