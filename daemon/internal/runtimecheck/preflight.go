package runtimecheck

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"carrier/daemon/internal/manifest"
)

const (
	CheckNameEnvVars          = "required_env_vars"
	CheckNamePortConflict     = "port_conflict"
	CheckNameCommandExists    = "command_exists"
	CheckNameRuntimePrereqs   = "runtime_prerequisites"

	IssueCodeEnvMissing       = "E_ENV_MISSING"
	IssueCodePortConflict     = "E_PORT_CONFLICT"
	IssueCodeCommandNotFound  = "E_COMMAND_NOT_FOUND"
)

// CheckResult represents a single pre-flight check outcome.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
	Repair  string `json:"repair,omitempty"`
}

// PreFlightResult aggregates all pre-flight check results.
type PreFlightResult struct {
	Passed bool          `json:"passed"`
	Checks []CheckResult `json:"checks"`
}

// PreFlightOption configures the pre-flight runner.
type PreFlightOption func(*preFlightRunner)

type preFlightRunner struct {
	checker    HostChecker
	getenv     func(string) string
	listenTCP  func(network, address string) (net.Listener, error)
	lookPath   func(string) (string, error)
}

// WithGetenv overrides os.Getenv for testing.
func WithGetenv(fn func(string) string) PreFlightOption {
	return func(r *preFlightRunner) { r.getenv = fn }
}

// WithListenTCP overrides net.Listen for testing.
func WithListenTCP(fn func(string, string) (net.Listener, error)) PreFlightOption {
	return func(r *preFlightRunner) { r.listenTCP = fn }
}

// WithCommandLookPath overrides exec.LookPath for testing.
func WithCommandLookPath(fn func(string) (string, error)) PreFlightOption {
	return func(r *preFlightRunner) { r.lookPath = fn }
}

// RunPreFlight executes all pre-flight checks against the given manifest and
// returns a structured PreFlightResult.
func RunPreFlight(m manifest.Manifest, checker HostChecker, opts ...PreFlightOption) PreFlightResult {
	r := &preFlightRunner{
		checker:   checker,
		getenv:    os.Getenv,
		listenTCP: net.Listen,
		lookPath:  exec.LookPath,
	}
	for _, opt := range opts {
		opt(r)
	}

	var checks []CheckResult

	// 1. Runtime prerequisites (existing checker logic)
	checks = append(checks, r.checkRuntimePrereqs(m))

	// 2. Required env vars
	checks = append(checks, r.checkEnvVars(m)...)

	// 3. Port conflicts
	checks = append(checks, r.checkPorts(m)...)

	// 4. Start command executable
	checks = append(checks, r.checkCommandExists(m))

	passed := true
	for _, c := range checks {
		if !c.Passed {
			passed = false
			break
		}
	}

	return PreFlightResult{Passed: passed, Checks: checks}
}

func (r *preFlightRunner) checkRuntimePrereqs(m manifest.Manifest) CheckResult {
	err := r.checker.Check(m)
	if err != nil {
		return CheckResult{
			Name:    CheckNameRuntimePrereqs,
			Passed:  false,
			Message: err.Error(),
			Code:    "E_RUNTIME_PREREQUISITES",
		}
	}
	return CheckResult{Name: CheckNameRuntimePrereqs, Passed: true, Message: "all runtime prerequisites met"}
}

func (r *preFlightRunner) checkEnvVars(m manifest.Manifest) []CheckResult {
	if len(m.Env.Required) == 0 {
		return []CheckResult{{Name: CheckNameEnvVars, Passed: true, Message: "no required env vars"}}
	}

	var missing []string
	for _, ev := range m.Env.Required {
		if strings.TrimSpace(r.getenv(ev.Name)) == "" {
			missing = append(missing, ev.Name)
		}
	}

	if len(missing) == 0 {
		return []CheckResult{{Name: CheckNameEnvVars, Passed: true, Message: "all required env vars set"}}
	}

	results := make([]CheckResult, 0, len(missing))
	for _, name := range missing {
		results = append(results, CheckResult{
			Name:    CheckNameEnvVars,
			Passed:  false,
			Code:    IssueCodeEnvMissing,
			Message: fmt.Sprintf("required environment variable %s is not set", name),
			Repair:  fmt.Sprintf("export %s=<value>", name),
		})
	}
	return results
}

func (r *preFlightRunner) checkPorts(m manifest.Manifest) []CheckResult {
	if len(m.Network.Ports) == 0 {
		return []CheckResult{{Name: CheckNamePortConflict, Passed: true, Message: "no ports declared"}}
	}

	var results []CheckResult
	for _, ps := range m.Network.Ports {
		if ps.Port <= 0 {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", ps.Port)
		ln, err := r.listenTCP("tcp", addr)
		if err != nil {
			results = append(results, CheckResult{
				Name:    CheckNamePortConflict,
				Passed:  false,
				Code:    IssueCodePortConflict,
				Message: fmt.Sprintf("port %d (%s) is already in use", ps.Port, ps.Name),
				Repair:  fmt.Sprintf("free port %d: lsof -ti:%d | xargs kill -9", ps.Port, ps.Port),
			})
		} else {
			_ = ln.Close()
			results = append(results, CheckResult{
				Name:    CheckNamePortConflict,
				Passed:  true,
				Message: fmt.Sprintf("port %d (%s) is available", ps.Port, ps.Name),
			})
		}
	}
	if len(results) == 0 {
		return []CheckResult{{Name: CheckNamePortConflict, Passed: true, Message: "no ports declared"}}
	}
	return results
}

func (r *preFlightRunner) checkCommandExists(m manifest.Manifest) CheckResult {
	cmd := strings.TrimSpace(m.Runtime.Start.Command)
	if cmd == "" {
		return CheckResult{
			Name:    CheckNameCommandExists,
			Passed:  false,
			Code:    IssueCodeCommandNotFound,
			Message: "runtime.start.command is empty",
		}
	}

	// Extract the first token (executable name)
	executable := strings.Fields(cmd)[0]

	_, err := r.lookPath(executable)
	if err != nil {
		return CheckResult{
			Name:    CheckNameCommandExists,
			Passed:  false,
			Code:    IssueCodeCommandNotFound,
			Message: fmt.Sprintf("executable %q not found in PATH", executable),
			Repair:  fmt.Sprintf("install %q or add its directory to PATH", executable),
		}
	}
	return CheckResult{
		Name:    CheckNameCommandExists,
		Passed:  true,
		Message: fmt.Sprintf("executable %q found", executable),
	}
}
