package main

import "testing"

func TestParseServiceCommandArgs(t *testing.T) {
	for _, action := range []string{"install", "start", "stop", "status", "uninstall"} {
		opts, err := parseServiceCommandArgs([]string{action})
		if err != nil {
			t.Fatalf("parseServiceCommandArgs(%s) error: %v", action, err)
		}
		if opts.Action != action {
			t.Fatalf("action = %q, want %q", opts.Action, action)
		}
	}
	if _, err := parseServiceCommandArgs([]string{"reload"}); err == nil {
		t.Fatal("expected unsupported action error")
	}
}
