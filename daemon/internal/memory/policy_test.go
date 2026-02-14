package memory

import (
	"testing"
)

func TestPolicyDefaultAccessMode(t *testing.T) {
	p := Policy{}
	tests := []struct {
		t    Type
		want AccessMode
	}{
		{TypePerAgent, AccessReadWrite},
		{TypeShared, AccessReadOnly},
		{TypePublic, AccessReadOnly},
	}
	for _, tc := range tests {
		if got := p.DefaultAccessMode(tc.t); got != tc.want {
			t.Errorf("DefaultAccessMode(%s) = %s, want %s", tc.t, got, tc.want)
		}
	}
}

func TestPolicyResolveAccessMode(t *testing.T) {
	p := Policy{}
	tests := []struct {
		memType   Type
		requested AccessMode
		want      AccessMode
	}{
		{TypePerAgent, AccessReadOnly, AccessReadWrite},   // always rw
		{TypePerAgent, AccessReadWrite, AccessReadWrite},
		{TypeShared, AccessReadOnly, AccessReadOnly},
		{TypeShared, AccessReadWrite, AccessReadWrite},     // explicit authorization
		{TypePublic, AccessReadOnly, AccessReadOnly},
		{TypePublic, AccessReadWrite, AccessReadOnly},      // forced ro
	}
	for _, tc := range tests {
		got := p.ResolveAccessMode(tc.memType, tc.requested)
		if got != tc.want {
			t.Errorf("ResolveAccessMode(%s, %s) = %s, want %s", tc.memType, tc.requested, got, tc.want)
		}
	}
}

func TestPolicyCheckMountArchivedDenied(t *testing.T) {
	p := Policy{}
	entry := Entry{ID: "m1", Type: TypeShared, State: StateArchived}
	err := p.CheckMount(entry, "agent-a", nil)
	if err == nil {
		t.Fatal("expected error for archived memory")
	}
}

func TestPolicyCheckMountMountedDenied(t *testing.T) {
	p := Policy{}
	entry := Entry{ID: "m1", Type: TypeShared, State: StateMounted}
	err := p.CheckMount(entry, "agent-a", nil)
	if err == nil {
		t.Fatal("expected error for already-mounted memory")
	}
}

func TestPolicyCheckMountDuplicateDenied(t *testing.T) {
	p := Policy{}
	entry := Entry{ID: "m1", Type: TypeShared, State: StateCreated}
	mounts := []MountRecord{{MemoryID: "m1", AgentID: "agent-a"}}
	err := p.CheckMount(entry, "agent-a", mounts)
	if err != ErrAlreadyMounted {
		t.Fatalf("expected ErrAlreadyMounted, got %v", err)
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		from, to State
		ok       bool
	}{
		{StateCreated, StateMounted, true},
		{StateCreated, StateArchived, true},
		{StateCreated, StateDetached, false},
		{StateMounted, StateDetached, true},
		{StateMounted, StateArchived, false},
		{StateDetached, StateMounted, true},
		{StateDetached, StateArchived, true},
		{StateArchived, StateMounted, false},
		{StateArchived, StateCreated, false},
	}
	for _, tc := range tests {
		err := ValidateTransition(tc.from, tc.to)
		if tc.ok && err != nil {
			t.Errorf("%s→%s should be valid, got %v", tc.from, tc.to, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s→%s should be invalid", tc.from, tc.to)
		}
	}
}
