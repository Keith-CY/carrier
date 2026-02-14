package runtimecheck

import (
	"errors"
	"strings"
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

	var preErr *PrerequisiteError
	if !errors.As(err, &preErr) {
		t.Fatalf("expected PrerequisiteError, got %T", err)
	}
	if len(preErr.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(preErr.Issues))
	}
	if preErr.Issues[0].Code != IssueCodeWSL2Missing {
		t.Fatalf("expected issue code %s, got %s", IssueCodeWSL2Missing, preErr.Issues[0].Code)
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

func TestGoRuntimeAddsWSLContextWhenWSLDetected(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		Lookup: LookupFunc(func(name string) (string, error) {
			if name == "go" {
				return "", errors.New("not found")
			}
			return "/bin/" + name, nil
		}),
		ReadFile: func(string) ([]byte, error) {
			return []byte("Linux version x.y.z-microsoft-standard-WSL2"), nil
		},
	}

	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeGoCLI}})
	if err == nil {
		t.Fatal("expected go prerequisite error")
	}
	if !strings.Contains(err.Error(), "WSL note") {
		t.Fatalf("expected WSL context in error, got %v", err)
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

func TestDetectWSLByMicrosoftMarker(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		ReadFile: func(string) ([]byte, error) {
			return []byte("Linux version x.y.z-microsoft-standard"), nil
		},
	}

	if !checker.detectWSL() {
		t.Fatal("expected WSL detection to be true for microsoft marker")
	}
}

func TestDetectWSLByWSLMarker(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		ReadFile: func(string) ([]byte, error) {
			return []byte("Linux version x.y.z WSL2"), nil
		},
	}

	if !checker.detectWSL() {
		t.Fatal("expected WSL detection to be true for WSL marker")
	}
}

func TestDetectWSLFalseOnReadError(t *testing.T) {
	checker := HostChecker{
		GOOS: "linux",
		ReadFile: func(string) ([]byte, error) {
			return nil, errors.New("read failed")
		},
	}

	if checker.detectWSL() {
		t.Fatal("expected WSL detection to be false on read error")
	}
}

func TestDetectWSLFalseOnNonLinux(t *testing.T) {
	checker := HostChecker{
		GOOS: "darwin",
		ReadFile: func(string) ([]byte, error) {
			return []byte("Linux version x.y.z-microsoft-standard"), nil
		},
	}

	if checker.detectWSL() {
		t.Fatal("expected WSL detection to be false on non-linux GOOS")
	}
}
