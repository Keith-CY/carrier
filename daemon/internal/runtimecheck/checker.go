package runtimecheck

import (
	"fmt"
	"os"
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

const (
	IssueCodeWSL2Missing      = "E_WSL2_MISSING"
	IssueCodeNPMMissing       = "E_NPM_MISSING"
	IssueCodeGoMissing        = "E_GO_MISSING"
	IssueCodeRuntimeTypeUnknown = "E_RUNTIME_TYPE_UNKNOWN"
)

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
	GOOS     string
	Lookup   ToolLookup
	ReadFile func(name string) ([]byte, error)
}

func NewHostChecker() HostChecker {
	return HostChecker{
		GOOS:     runtime.GOOS,
		Lookup:   LookupFunc(exec.LookPath),
		ReadFile: os.ReadFile,
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
	isWSL := c.detectWSL()

	if c.GOOS == "windows" {
		if !c.hasTool("wsl.exe") {
			issues = append(issues, Issue{
				Code:    IssueCodeWSL2Missing,
				Message: "Windows runtime requires WSL2 (wsl.exe not found)",
			})
		}
		return issues
	}

	switch runtimeType {
	case manifest.RuntimeTypeNpmCLI:
		if !c.hasTool("npm") {
			msg := "npm is required for runtime.type=npm_cli"
			if isWSL {
				msg += " (WSL note: install npm in your Linux distro, not Windows)"
			}
			issues = append(issues, Issue{
				Code:    IssueCodeNPMMissing,
				Message: msg,
			})
		}
	case manifest.RuntimeTypeGoCLI:
		if !c.hasTool("go") {
			msg := "go is required for runtime.type=go_cli"
			if isWSL {
				msg += " (WSL note: install Go in your Linux distro and ensure /usr/local/go/bin is on PATH)"
			}
			issues = append(issues, Issue{
				Code:    IssueCodeGoMissing,
				Message: msg,
			})
		}
	case manifest.RuntimeTypeLocalBinary:
		// No extra host tool requirement.
	default:
		issues = append(issues, Issue{
			Code:    IssueCodeRuntimeTypeUnknown,
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

func (c HostChecker) detectWSL() bool {
	if c.GOOS != "linux" || c.ReadFile == nil {
		return false
	}

	b, err := c.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	version := strings.ToLower(string(b))
	return strings.Contains(version, "microsoft") || strings.Contains(version, "wsl")
}
