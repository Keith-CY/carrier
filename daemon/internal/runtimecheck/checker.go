package runtimecheck

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"carrier/daemon/internal/manifest"
)

type ToolLookup interface {
	LookPath(file string) (string, error)
}

type LookupFunc func(string) (string, error)

func (f LookupFunc) LookPath(file string) (string, error) {
	return f(file)
}

type Checker interface {
	Check(m manifest.Manifest) error
}

type Issue struct {
	Code    string
	Message string
}

type PrerequisiteError struct {
	Issues []Issue
}

func (e *PrerequisiteError) Error() string {
	if len(e.Issues) == 0 {
		return "runtime prerequisites failed"
	}

	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Code, issue.Message))
	}

	return "runtime prerequisites failed: " + strings.Join(parts, "; ")
}

type HostChecker struct {
	GOOS   string
	Lookup ToolLookup
}

func NewHostChecker() HostChecker {
	return HostChecker{
		GOOS:   runtime.GOOS,
		Lookup: LookupFunc(exec.LookPath),
	}
}

func (c HostChecker) Check(m manifest.Manifest) error {
	issues := c.collectIssues(m.Runtime.Type)
	if len(issues) == 0 {
		return nil
	}

	return &PrerequisiteError{Issues: issues}
}

func (c HostChecker) collectIssues(runtimeType manifest.RuntimeType) []Issue {
	issues := make([]Issue, 0, 2)

	if c.GOOS == "windows" {
		if !c.hasTool("wsl.exe") {
			issues = append(issues, Issue{
				Code:    "E_WSL2_MISSING",
				Message: "Windows runtime requires WSL2 (wsl.exe not found)",
			})
		}
		return issues
	}

	switch runtimeType {
	case manifest.RuntimeTypeNpmCLI:
		if !c.hasTool("npm") {
			issues = append(issues, Issue{
				Code:    "E_NPM_MISSING",
				Message: "npm is required for runtime.type=npm_cli",
			})
		}
	case manifest.RuntimeTypeGoCLI:
		if !c.hasTool("go") {
			issues = append(issues, Issue{
				Code:    "E_GO_MISSING",
				Message: "go is required for runtime.type=go_cli",
			})
		}
	case manifest.RuntimeTypeLocalBinary:
		// No extra host tool requirement.
	default:
		issues = append(issues, Issue{
			Code:    "E_RUNTIME_TYPE_UNKNOWN",
			Message: fmt.Sprintf("unsupported runtime type: %s", runtimeType),
		})
	}

	return issues
}

func (c HostChecker) hasTool(name string) bool {
	if c.Lookup == nil {
		return false
	}
	_, err := c.Lookup.LookPath(name)
	return err == nil
}
