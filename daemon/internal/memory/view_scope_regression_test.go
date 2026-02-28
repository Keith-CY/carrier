package memory

import "testing"

func hasScope(scopes []Scope, target Scope) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func TestDetachMemoryPreservesManualScopeWhenScopeOverlapsAttachment(t *testing.T) {
	s := newTestStore()
	if _, err := s.Create("m1", "shared memory", "1.0.0", TypeShared, ""); err != nil {
		t.Fatalf("create shared memory: %v", err)
	}
	if err := s.AttachScope("inst-1", Scope("shared:default")); err != nil {
		t.Fatalf("attach manual scope: %v", err)
	}
	if _, err := s.AttachMemory("inst-1", "m1", AttachOptions{}); err != nil {
		t.Fatalf("attach memory: %v", err)
	}
	if err := s.DetachMemory("inst-1", "m1"); err != nil {
		t.Fatalf("detach memory: %v", err)
	}

	scopes := s.InstanceScopes("inst-1")
	if len(scopes) != 1 || scopes[0] != Scope("shared:default") {
		t.Fatalf("expected manual scope to remain after detach, got %+v", scopes)
	}
}

func TestSetAttachmentsFromLinksPreservesManualScopes(t *testing.T) {
	s := newTestStore()
	if _, err := s.Create("m1", "shared memory", "1.0.0", TypeShared, ""); err != nil {
		t.Fatalf("create shared memory: %v", err)
	}
	if err := s.AttachScope("inst-1", Scope("shared:team")); err != nil {
		t.Fatalf("attach manual scope: %v", err)
	}

	if err := s.SetAttachmentsFromLinks("inst-1", []string{"m1"}); err != nil {
		t.Fatalf("set attachments: %v", err)
	}
	scopes := s.InstanceScopes("inst-1")
	if len(scopes) != 2 || !hasScope(scopes, Scope("shared:default")) || !hasScope(scopes, Scope("shared:team")) {
		t.Fatalf("expected merged manual+derived scopes, got %+v", scopes)
	}

	if err := s.SetAttachmentsFromLinks("inst-1", nil); err != nil {
		t.Fatalf("clear attachments: %v", err)
	}
	scopes = s.InstanceScopes("inst-1")
	if len(scopes) != 1 || scopes[0] != Scope("shared:team") {
		t.Fatalf("expected manual scope to remain after attachment sync, got %+v", scopes)
	}
}
