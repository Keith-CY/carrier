package runtimecheck

import (
	"errors"
	"net"
	"testing"

	"carrier/daemon/internal/manifest"
)

func newPassingChecker() HostChecker {
	return HostChecker{
		GOOS: "linux",
		Lookup: LookupFunc(func(string) (string, error) {
			return "/usr/bin/x", nil
		}),
		ReadFile: func(string) ([]byte, error) {
			return []byte("Linux version x.y.z-generic"), nil
		},
	}
}

func baseManifest() manifest.Manifest {
	return manifest.Manifest{
		Runtime: manifest.RuntimeSpec{
			Type:  manifest.RuntimeTypeLocalBinary,
			Start: manifest.CommandSpec{Command: "myagent serve"},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{{Name: "API_KEY"}},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 18080}},
		},
	}
}

func TestRunPreFlight_AllPass(t *testing.T) {
	m := baseManifest()
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(name string) string {
			if name == "API_KEY" {
				return "secret"
			}
			return ""
		}),
		WithListenTCP(func(_, _ string) (net.Listener, error) {
			return &fakeListener{}, nil
		}),
		WithCommandLookPath(func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		}),
	)

	if !result.Passed {
		t.Fatalf("expected all checks to pass, got failures: %+v", result.Checks)
	}
}

func TestRunPreFlight_MissingEnvVar(t *testing.T) {
	m := baseManifest()
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "" }),
		WithListenTCP(func(_, _ string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/x", nil }),
	)

	if result.Passed {
		t.Fatal("expected failure due to missing env var")
	}
	found := false
	for _, c := range result.Checks {
		if c.Code == IssueCodeEnvMissing {
			found = true
			if c.Repair == "" {
				t.Error("expected repair hint for missing env var")
			}
		}
	}
	if !found {
		t.Fatal("expected E_ENV_MISSING check result")
	}
}

func TestRunPreFlight_PortConflict(t *testing.T) {
	m := baseManifest()
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "val" }),
		WithListenTCP(func(_, _ string) (net.Listener, error) {
			return nil, errors.New("address already in use")
		}),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/x", nil }),
	)

	if result.Passed {
		t.Fatal("expected failure due to port conflict")
	}
	found := false
	for _, c := range result.Checks {
		if c.Code == IssueCodePortConflict {
			found = true
			if c.Repair == "" {
				t.Error("expected repair hint for port conflict")
			}
		}
	}
	if !found {
		t.Fatal("expected E_PORT_CONFLICT check result")
	}
}

func TestRunPreFlight_CommandNotFound(t *testing.T) {
	m := baseManifest()
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "val" }),
		WithListenTCP(func(_, _ string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) {
			return "", errors.New("not found")
		}),
	)

	if result.Passed {
		t.Fatal("expected failure due to missing command")
	}
	found := false
	for _, c := range result.Checks {
		if c.Code == IssueCodeCommandNotFound {
			found = true
		}
	}
	if !found {
		t.Fatal("expected E_COMMAND_NOT_FOUND check result")
	}
}

func TestRunPreFlight_NoRequiredEnvVars(t *testing.T) {
	m := baseManifest()
	m.Env.Required = nil
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "" }),
		WithListenTCP(func(_, _ string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/x", nil }),
	)

	if !result.Passed {
		t.Fatalf("expected pass with no required env vars, got: %+v", result.Checks)
	}
}

func TestRunPreFlight_NoPorts(t *testing.T) {
	m := baseManifest()
	m.Network.Ports = nil
	checker := newPassingChecker()

	result := RunPreFlight(m, checker,
		WithGetenv(func(string) string { return "val" }),
		WithListenTCP(func(_, _ string) (net.Listener, error) { return &fakeListener{}, nil }),
		WithCommandLookPath(func(string) (string, error) { return "/usr/bin/x", nil }),
	)

	if !result.Passed {
		t.Fatalf("expected pass with no ports, got: %+v", result.Checks)
	}
}

func TestHostChecker_PreFlight(t *testing.T) {
	checker := newPassingChecker()
	m := manifest.Manifest{
		Runtime: manifest.RuntimeSpec{
			Type:  manifest.RuntimeTypeLocalBinary,
			Start: manifest.CommandSpec{Command: "ls"},
		},
	}
	result := checker.PreFlight(m)
	if !result.Passed {
		t.Fatalf("expected pass, got: %+v", result.Checks)
	}
}

// fakeListener implements net.Listener for testing.
type fakeListener struct{}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f *fakeListener) Close() error               { return nil }
func (f *fakeListener) Addr() net.Addr              { return &net.TCPAddr{} }
