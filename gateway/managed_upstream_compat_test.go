package gateway

import "testing"

func TestSemverSatisfiesRange(t *testing.T) {
	cases := []struct {
		name    string
		version string
		rangeEx string
		want    bool
	}{
		{name: "within", version: "0.1.7", rangeEx: ">=0.1.0 <1.0.0", want: true},
		{name: "lower-bound", version: "0.1.0", rangeEx: ">=0.1.0 <1.0.0", want: true},
		{name: "too-low", version: "0.0.9", rangeEx: ">=0.1.0 <1.0.0", want: false},
		{name: "too-high", version: "1.0.0", rangeEx: ">=0.1.0 <1.0.0", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := semverSatisfiesRange(tc.version, tc.rangeEx)
			if err != nil {
				t.Fatalf("semverSatisfiesRange(%q,%q) error: %v", tc.version, tc.rangeEx, err)
			}
			if got != tc.want {
				t.Fatalf("semverSatisfiesRange(%q,%q)=%v, want %v", tc.version, tc.rangeEx, got, tc.want)
			}
		})
	}
}

func TestResolveManagedRenderer_UsesEnvOverride(t *testing.T) {
	resetManagedCompatLockForTests()
	t.Cleanup(resetManagedCompatLockForTests)
	t.Setenv("CARRIER_MANAGED_ZEROCLAW_VERSION", "0.1.7")

	selection, err := resolveManagedRenderer("zeroclaw")
	if err != nil {
		t.Fatalf("resolveManagedRenderer: %v", err)
	}
	if selection.RendererID != "zeroclaw.toml.v1" {
		t.Fatalf("renderer=%q, want zeroclaw.toml.v1", selection.RendererID)
	}
	if selection.ConfigFormat != "toml" {
		t.Fatalf("format=%q, want toml", selection.ConfigFormat)
	}
	if selection.AgentVersion != "0.1.7" {
		t.Fatalf("version=%q, want 0.1.7", selection.AgentVersion)
	}
}

func TestResolveManagedRenderer_InvalidEnvOverride(t *testing.T) {
	resetManagedCompatLockForTests()
	t.Cleanup(resetManagedCompatLockForTests)
	t.Setenv("CARRIER_MANAGED_ZEROCLAW_VERSION", "invalid-version")

	if _, err := resolveManagedRenderer("zeroclaw"); err == nil {
		t.Fatal("expected error for invalid env override")
	}
}
