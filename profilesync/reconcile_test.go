package profilesync

import "testing"

func TestReconcileProfilesLocalPriority(t *testing.T) {
	t.Parallel()

	base := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-4.1",
			},
		},
	}
	local := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-5",
			},
		},
	}
	remote := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": "gpt-4.1-mini",
			},
		},
	}

	report := ReconcileProfiles(base, local, remote, ReconcileOptions{
		ConflictPolicy: ConflictPolicyPreferLocal,
	})
	if report.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got %d", report.ConflictCount)
	}
	model := nestedString(report.ReconciledProfile, "agents", "defaults", "model")
	if model != "gpt-5" {
		t.Fatalf("expected local model to win, got %q", model)
	}
}

func TestReconcileProfilesAcceptRemoteWhenLocalUnchanged(t *testing.T) {
	t.Parallel()

	base := map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"base_url": "https://api.openai.com/v1",
			},
		},
	}
	local := map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"base_url": "https://api.openai.com/v1",
			},
		},
	}
	remote := map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"base_url": "https://proxy.example.com/v1",
			},
		},
	}

	report := ReconcileProfiles(base, local, remote, ReconcileOptions{
		ConflictPolicy: ConflictPolicyPreferLocal,
	})
	if report.AcceptedRemoteCount != 1 {
		t.Fatalf("expected one accepted remote change, got %d", report.AcceptedRemoteCount)
	}
	baseURL := nestedString(report.ReconciledProfile, "providers", "openai", "base_url")
	if baseURL != "https://proxy.example.com/v1" {
		t.Fatalf("expected accepted remote base_url, got %q", baseURL)
	}
}
