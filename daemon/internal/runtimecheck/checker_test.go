package runtimecheck

import (
	"errors"
	"testing"

	"carrier/daemon/internal/manifest"
)

func TestWindowsRequiresWSL(t *testing.T) {
	checker := HostChecker{
		GOOS: "windows",
		Lookup: LookupFunc(func(string) (string, error) {
			return "", errors.New("not found")
		}),
	}

	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeLocalBinary}})
	if err == nil {
		t.Fatal("expected missing WSL prerequisite error")
	}
}

func TestNpmRuntimeRequiresNpmOnMacLinux(t *testing.T) {
	checker := HostChecker{
		GOOS: "darwin",
		Lookup: LookupFunc(func(name string) (string, error) {
			if name == "npm" {
				return "", errors.New("not found")
			}
			return "/bin/" + name, nil
		}),
	}

	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeNpmCLI}})
	if err == nil {
		t.Fatal("expected npm prerequisite error")
	}
}

func TestGoRuntimeRequiresGoOnMacLinux(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		Lookup: LookupFunc(func(name string) (string, error) {
			if name == "go" {
				return "", errors.New("not found")
			}
			return "/bin/" + name, nil
		}),
	}

	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeGoCLI}})
	if err == nil {
		t.Fatal("expected go prerequisite error")
	}
}

func TestLocalBinaryPassesWithoutToolingOnLinux(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		Lookup: LookupFunc(func(name string) (string, error) {
			return "/bin/" + name, nil
		}),
	}

	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeLocalBinary}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
