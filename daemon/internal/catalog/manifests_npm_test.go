package catalog

import (
	"carrier/daemon/internal/manifest"
	"strings"
	"testing"
)

func TestNpmAgentManifests(t *testing.T) {
	for _, spec := range npmAgentSpecs {
		t.Run(spec.ID, func(t *testing.T) {
			m := BuildNpmAgentManifest(spec)
			if err := m.Validate(); err != nil {
				t.Fatalf("manifest validation failed: %v", err)
			}
			if m.Runtime.Type != manifest.RuntimeTypeNpmCLI {
				t.Fatalf("runtime.type = %q, want %q", m.Runtime.Type, manifest.RuntimeTypeNpmCLI)
			}
			if m.ID != spec.ID {
				t.Fatalf("manifest id = %q, want %q", m.ID, spec.ID)
			}

			installCmd, err := m.Runtime.Install.ResolveForGOOS(manifest.CommandOSLinux)
			if err != nil {
				t.Fatalf("resolve linux install command: %v", err)
			}
			if !strings.Contains(installCmd, spec.NpmPackage) {
				t.Fatalf("install command should reference %q, got %q", spec.NpmPackage, installCmd)
			}
			if !strings.Contains(installCmd, "command -v "+spec.BinaryName) {
				t.Fatalf("install command should verify binary %q, got %q", spec.BinaryName, installCmd)
			}

			stopCmd, err := m.Runtime.Stop.ResolveForCurrentOS()
			if err != nil {
				t.Fatalf("resolve stop command: %v", err)
			}
			if stopCmd != "signal:term" {
				t.Fatalf("stop command = %q, want signal:term", stopCmd)
			}
		})
	}
}

func TestNpmAgentManifestsByPlatform(t *testing.T) {
	platforms := []string{manifest.CommandOSLinux, manifest.CommandOSDarwin, manifest.CommandOSWindows}
	for _, spec := range npmAgentSpecs {
		for _, goos := range platforms {
			t.Run(spec.ID+"/"+goos, func(t *testing.T) {
				m := BuildNpmAgentManifest(spec)
				cmd, err := m.Runtime.Install.ResolveForGOOS(goos)
				if err != nil {
					t.Fatalf("resolve install command for %s: %v", goos, err)
				}
				if strings.TrimSpace(cmd) == "" {
					t.Fatal("install command should not be empty")
				}
			})
		}
	}
}
