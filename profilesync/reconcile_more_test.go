package profilesync

import (
	"math"
	"reflect"
	"testing"
)

func TestReconcileProfilesConflictPolicyPrefersRemote(t *testing.T) {
	t.Parallel()

	base := map[string]interface{}{
		"service": map[string]interface{}{
			"provider": "openclaw",
			"endpoint": "https://api.openai.com/v1",
		},
	}
	local := map[string]interface{}{
		"service": map[string]interface{}{
			"provider": "openclaw",
			"endpoint": "http://127.0.0.1:11434",
		},
	}
	remote := map[string]interface{}{
		"service": map[string]interface{}{
			"provider": "openclaw",
			"endpoint": "https://api.groq.com/openai/v1",
		},
	}

	report := ReconcileProfiles(base, local, remote, ReconcileOptions{
		ConflictPolicy: ConflictPolicyPreferRemote,
	})
	if report.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got %d", report.ConflictCount)
	}

	reconciled := nestedString(report.ReconciledProfile, "service", "endpoint")
	if reconciled != "https://api.groq.com/openai/v1" {
		t.Fatalf("expected reconciled endpoint from remote, got %q", reconciled)
	}
}

func TestReconcileProfilesNestedMapAndAcceptedRemotePath(t *testing.T) {
	t.Parallel()

	base := map[string]interface{}{
		"outer": map[string]interface{}{
			"nested": map[string]interface{}{
				"active": "true",
				"ports":  map[string]interface{}{"http": 443},
			},
		},
	}
	local := map[string]interface{}{
		"outer": map[string]interface{}{
			"nested": map[string]interface{}{
				"active": "true",
				"ports":  map[string]interface{}{"http": 443},
			},
		},
	}
	remote := map[string]interface{}{
		"outer": map[string]interface{}{
			"nested": map[string]interface{}{
				"active": "false",
				"ports":  map[string]interface{}{"http": 443, "metrics": 9090},
			},
		},
	}

	report := ReconcileProfiles(base, local, remote, ReconcileOptions{})
	if report.AcceptedRemoteCount != 1 {
		t.Fatalf("expected 1 accepted remote change, got %d", report.AcceptedRemoteCount)
	}
	if got := nestedString(report.ReconciledProfile, "outer", "nested", "active"); got != "false" {
		t.Fatalf("expected active to follow remote, got %q", got)
	}
}

func TestNestedStringCornerCases(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"a": map[string]interface{}{
			"b": "value",
			"c": 123,
		},
	}
	if got := nestedString(payload, "a"); got != "" {
		t.Fatalf("expected empty string for short path, got %q", got)
	}
	if got := nestedString(payload, "missing", "x"); got != "" {
		t.Fatalf("expected empty string for missing path, got %q", got)
	}
	if got := nestedString(payload, "a", "c"); got != "" {
		t.Fatalf("expected empty string for non-string leaf, got %q", got)
	}
}

func TestUnionKeysDeterministicOrdering(t *testing.T) {
	t.Parallel()

	got := unionKeys(
		map[string]interface{}{
			"zeta":  "1",
			"alpha": "2",
		},
		map[string]interface{}{
			"beta":  "3",
			"alpha": "4",
		},
	)
	want := []string{"alpha", "beta", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sorted keys: got=%v want=%v", got, want)
	}
}

func TestDeepCopyMapFallbackOnMarshalError(t *testing.T) {
	t.Parallel()

	original := map[string]interface{}{
		"provider": "openclaw",
		"rate":     math.Inf(1),
	}
	copied := deepCopyMap(original)
	if copied["provider"] != "openclaw" {
		t.Fatalf("expected provider to be copied, got %v", copied["provider"])
	}
	if copied["rate"] == nil || copied["rate"].(float64) != math.Inf(1) {
		t.Fatalf("expected inf value to remain after marshal fallback copy, got %v", copied["rate"])
	}
}

func TestAsMapAndUnionKeysCoverage(t *testing.T) {
	t.Parallel()

	if m, ok := asMap(map[string]interface{}(nil)); !ok {
		t.Fatalf("expected nil map value to be recognized as map, got ok=%v", ok)
	} else if len(m) != 0 {
		t.Fatalf("expected empty map for nil map input, got %v", m)
	}

	if _, ok := asMap(struct{}{}); ok {
		t.Fatal("expected struct to not be recognized as map")
	}

	if _, ok := asMap(map[string]interface{}{"a": 1}); !ok {
		t.Fatal("expected non-empty map to be recognized as map")
	}
}
