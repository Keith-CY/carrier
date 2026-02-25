package runtimecheck

import "testing"

func TestNewHostCheckerInitializesDependencies(t *testing.T) {
	c := NewHostChecker()
	if c.GOOS == "" {
		t.Fatal("expected GOOS to be set")
	}
	if c.Lookup == nil {
		t.Fatal("expected Lookup to be initialized")
	}
	if c.ReadFile == nil {
		t.Fatal("expected ReadFile to be initialized")
	}
}
