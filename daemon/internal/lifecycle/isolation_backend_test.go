package lifecycle

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveIsolationBackendLinuxInstallsBubblewrapWhenMissing(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	origRun := isolationBackendRun
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
		isolationBackendRun = origRun
	})

	isolationRuntimeGOOS = "linux"
	isolationEnvLookup = func(string) string { return "" }
	bwrapLookups := 0
	isolationBackendLookup = func(name string) (string, error) {
		switch name {
		case "bwrap":
			bwrapLookups++
			if bwrapLookups == 1 {
				return "", errors.New("not found")
			}
			return "/usr/bin/bwrap", nil
		case "apt-get":
			return "/usr/bin/apt-get", nil
		case "sudo":
			return "/usr/bin/sudo", nil
		default:
			return "", errors.New("not found")
		}
	}

	var calls []string
	isolationBackendRun = func(command string, args ...string) error {
		calls = append(calls, command+" "+strings.Join(args, " "))
		return nil
	}

	backend, err := resolveIsolationBackend()
	if err != nil {
		t.Fatalf("resolveIsolationBackend: %v", err)
	}
	linuxBackend, ok := backend.(linuxIsolationBackend)
	if !ok {
		t.Fatalf("backend type = %T, want linuxIsolationBackend", backend)
	}
	if linuxBackend.bwrapPath != "/usr/bin/bwrap" {
		t.Fatalf("bwrapPath = %q, want /usr/bin/bwrap", linuxBackend.bwrapPath)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one install command, got %d (%v)", len(calls), calls)
	}
	if calls[0] != "sudo apt-get install -y bubblewrap" {
		t.Fatalf("install command = %q, want %q", calls[0], "sudo apt-get install -y bubblewrap")
	}
}

func TestResolveIsolationBackendLinuxReturnsHelpfulErrorWhenAutoInstallUnavailable(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	origRun := isolationBackendRun
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
		isolationBackendRun = origRun
	})

	isolationRuntimeGOOS = "linux"
	isolationEnvLookup = func(string) string { return "" }
	isolationBackendLookup = func(name string) (string, error) {
		if name == "bwrap" {
			return "", errors.New("not found")
		}
		return "", errors.New("not found")
	}
	isolationBackendRun = func(command string, args ...string) error {
		t.Fatalf("unexpected install command %s %v", command, args)
		return nil
	}

	_, err := resolveIsolationBackend()
	if err == nil {
		t.Fatal("expected resolveIsolationBackend to fail")
	}
	got := err.Error()
	if !strings.Contains(got, "bubblewrap (bwrap) executable not found in PATH") {
		t.Fatalf("expected original missing-bwrap error, got %q", got)
	}
	if !strings.Contains(got, "automatic install attempt failed") {
		t.Fatalf("expected auto-install failure context, got %q", got)
	}
	if !strings.Contains(got, "no supported package manager found") {
		t.Fatalf("expected missing package manager context, got %q", got)
	}
}
