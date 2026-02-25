package runtimecheck

import (
	"errors"
	"net"
	"strings"
	"testing"

	"carrier/daemon/internal/manifest"
)

func TestPrerequisiteError_EmptyIssuesMessage(t *testing.T) {
	err := (&PrerequisiteError{}).Error()
	if err != "runtime prerequisites failed" {
		t.Fatalf("unexpected empty issues message: %q", err)
	}
}

func TestHostChecker_UnknownRuntimeType(t *testing.T) {
	checker := HostChecker{GOOS: "linux", Lookup: LookupFunc(func(string) (string, error) { return "/bin/x", nil })}
	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeType("mystery")}})
	if err == nil {
		t.Fatal("expected unknown runtime error")
	}
	if !strings.Contains(err.Error(), IssueCodeUnknownRuntime) {
		t.Fatalf("expected %s in error, got %v", IssueCodeUnknownRuntime, err)
	}
}

func TestHostChecker_NilLookupTriggersMissingToolIssue(t *testing.T) {
	checker := HostChecker{GOOS: "linux", Lookup: nil}
	err := checker.Check(manifest.Manifest{Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeNpmCLI}})
	if err == nil {
		t.Fatal("expected npm prerequisite error when Lookup is nil")
	}
	if !strings.Contains(err.Error(), IssueCodeNPMMissing) {
		t.Fatalf("expected %s in error, got %v", IssueCodeNPMMissing, err)
	}
}

func TestRunPreFlight_RuntimePrereqsFailureBranch(t *testing.T) {
	m := manifest.Manifest{
		Runtime: manifest.RuntimeSpec{
			Type:  manifest.RuntimeTypeNpmCLI,
			Start: manifest.CommandSpec{Command: "sh -c true"},
		},
	}
	checker := HostChecker{GOOS: "linux", Lookup: nil}

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "" }),
		WithListenTCP(func(string, string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) { return "/bin/sh", nil }),
	)
	if result.Passed {
		t.Fatalf("expected preflight failure, got %+v", result.Checks)
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == CheckNameRuntimePrereqs {
			found = true
			if c.Passed || c.Code != "E_RUNTIME_PREREQUISITES" {
				t.Fatalf("unexpected runtime prereq check: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected %s check in results: %+v", CheckNameRuntimePrereqs, result.Checks)
	}
}

func TestRunPreFlight_NonPositivePortsProduceNoPortChecks(t *testing.T) {
	m := manifest.Manifest{
		Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeLocalBinary, Start: manifest.CommandSpec{Command: "ls"}},
		Network: manifest.NetworkSpec{Ports: []manifest.PortSpec{{Name: "invalid-neg", Port: -1}, {Name: "invalid-zero", Port: 0}}},
	}
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "" }),
		WithListenTCP(func(string, string) (net.Listener, error) {
			return nil, errors.New("should not be called for non-positive ports")
		}),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/ls", nil }),
	)
	if !result.Passed {
		t.Fatalf("expected pass, got %+v", result.Checks)
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == CheckNamePortConflict {
			found = true
			if !c.Passed || !strings.Contains(c.Message, "no ports declared") {
				t.Fatalf("unexpected port check result: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected %s check in results: %+v", CheckNamePortConflict, result.Checks)
	}
}

func TestRunPreFlight_CommandResolveErrorBranch(t *testing.T) {
	m := manifest.Manifest{
		Runtime: manifest.RuntimeSpec{Type: manifest.RuntimeTypeLocalBinary, Start: manifest.CommandSpec{}},
	}
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "" }),
		WithListenTCP(func(string, string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/x", nil }),
	)
	if result.Passed {
		t.Fatalf("expected command resolution failure, got %+v", result.Checks)
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == CheckNameCommandExists {
			found = true
			if c.Passed || c.Code != IssueCodeCommandNotFound {
				t.Fatalf("unexpected command check result: %+v", c)
			}
			if !strings.Contains(c.Message, "runtime.start command is unavailable") {
				t.Fatalf("unexpected command check message: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected %s check in results: %+v", CheckNameCommandExists, result.Checks)
	}
}
