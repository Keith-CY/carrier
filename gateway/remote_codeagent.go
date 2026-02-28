package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"carrier/codeagent/adapters/codex"
	"carrier/codeagent/adapters/opencode"
	"carrier/codeagent/contract"
	"carrier/codeagent/policy"
	"carrier/codeagent/runtime"
)

type remoteCodeAgentRequest struct {
	Backend       string                 `json:"backend"`
	WorkspaceRoot string                 `json:"workspaceRoot"`
	Capability    string                 `json:"capability"`
	Path          string                 `json:"path"`
	Content       string                 `json:"content"`
	WriteMode     string                 `json:"writeMode"`
	Command       string                 `json:"command"`
	CWD           string                 `json:"cwd"`
	TimeoutSec    int                    `json:"timeoutSec"`
	StdoutPath    string                 `json:"stdoutPath"`
	StderrPath    string                 `json:"stderrPath"`
	AppendOutput  bool                   `json:"appendOutput"`
	ResumeSession string                 `json:"resumeSessionId"`
	Profile       map[string]interface{} `json:"profile"`
}

func handleRemoteCodeAgent(w http.ResponseWriter, r *http.Request, requestID string, host RemoteHost, hostID, agentID string, parts []string) {
	if len(parts) < 5 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "codeagent action path is required"))
		return
	}
	action := strings.ToLower(strings.TrimSpace(parts[4]))
	switch action {
	case "install":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req remoteCodeAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		backend := resolveCodeAgentBackend(req.Backend)
		workspace := normalizeWorkspaceRoot(req.WorkspaceRoot)
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
		defer cancel()
		adapter, err := newRemoteCodeAgentAdapter(host, backend)
		if err == nil {
			err = remoteInstallCodeAgentBinary(ctx, host, backend)
		}
		if err == nil {
			err = adapter.Install(ctx, contract.Target{HostID: hostID, WorkspaceRoot: workspace})
		}
		if err == nil {
			err = adapter.Health(ctx)
		}
		version := ""
		if err == nil {
			version, err = adapter.Version(ctx)
		}
		recordRemoteOperationMetric(remoteOpCodeAgentInstall, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_codeagent_install", hostID+":"+agentID, "failure", map[string]interface{}{
				"backend": backend,
				"error":   err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_INSTALL_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_codeagent_install", hostID+":"+agentID, "success", map[string]interface{}{
			"backend": backend,
			"version": version,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"install": map[string]interface{}{
				"hostId":        hostID,
				"agentId":       agentID,
				"backend":       backend,
				"workspaceRoot": workspace,
				"installed":     true,
				"version":       version,
			},
		})
		return
	case "configure":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req remoteCodeAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		backend := resolveCodeAgentBackend(req.Backend)
		workspace := normalizeWorkspaceRoot(req.WorkspaceRoot)
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		adapter, err := newRemoteCodeAgentAdapter(host, backend)
		if err == nil {
			err = adapter.Configure(ctx, contract.Target{HostID: hostID, WorkspaceRoot: workspace}, contract.Profile{
				Name:   backend,
				Values: req.Profile,
			})
		}
		recordRemoteOperationMetric(remoteOpCodeAgentConfigure, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_codeagent_configure", hostID+":"+agentID, "failure", map[string]interface{}{
				"backend": backend,
				"error":   err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_CONFIGURE_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_codeagent_configure", hostID+":"+agentID, "success", map[string]interface{}{
			"backend": backend,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"configure": map[string]interface{}{
				"backend":       backend,
				"workspaceRoot": workspace,
				"configured":    true,
			},
		})
		return
	case "health":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		backend := resolveCodeAgentBackend(r.URL.Query().Get("backend"))
		workspace := normalizeWorkspaceRoot(r.URL.Query().Get("workspaceRoot"))
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		adapter, err := newRemoteCodeAgentAdapter(host, backend)
		if err == nil {
			err = adapter.Health(ctx)
		}
		recordRemoteOperationMetric(remoteOpCodeAgentHealth, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_codeagent_health", hostID+":"+agentID, "failure", map[string]interface{}{
				"backend": backend,
				"error":   err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_HEALTH_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_codeagent_health", hostID+":"+agentID, "success", map[string]interface{}{
			"backend": backend,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"health": map[string]interface{}{
				"backend":       backend,
				"workspaceRoot": workspace,
				"healthy":       true,
			},
		})
		return
	case "version":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		backend := resolveCodeAgentBackend(r.URL.Query().Get("backend"))
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		adapter, err := newRemoteCodeAgentAdapter(host, backend)
		version := ""
		if err == nil {
			version, err = adapter.Version(ctx)
		}
		recordRemoteOperationMetric(remoteOpCodeAgentVersion, startedAt, err)
		if err != nil {
			emitRemoteAuditEvent(requestID, "remote_codeagent_version", hostID+":"+agentID, "failure", map[string]interface{}{
				"backend": backend,
				"error":   err.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_VERSION_FAILED", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "remote_codeagent_version", hostID+":"+agentID, "success", map[string]interface{}{
			"backend": backend,
			"version": version,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"version": map[string]interface{}{
				"backend": backend,
				"value":   version,
			},
		})
		return
	case "run":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req remoteCodeAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		backend := resolveCodeAgentBackend(req.Backend)
		workspace := normalizeWorkspaceRoot(req.WorkspaceRoot)
		capability, err := parseCodeAgentCapability(req.Capability)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		adapter, err := newRemoteCodeAgentAdapter(host, backend)
		if err != nil {
			recordRemoteOperationMetric(remoteOpCodeAgentRun, startedAt, err)
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_RUN_FAILED", err.Error()))
			return
		}
		strict := policy.NewStrictPolicy(workspace)
		orch := runtime.NewOrchestrator(adapter, []runtime.Middleware{
			func(_ context.Context, runReq contract.RunRequest) contract.PolicyDecisionEnvelope {
				return strict.Decide(runReq)
			},
		})
		out, runErr := orch.Run(ctx, contract.RunRequest{
			Capability:      capability,
			Path:            strings.TrimSpace(req.Path),
			Content:         req.Content,
			WriteMode:       contract.WriteMode(strings.TrimSpace(req.WriteMode)),
			Command:         strings.TrimSpace(req.Command),
			CWD:             strings.TrimSpace(req.CWD),
			TimeoutSec:      req.TimeoutSec,
			StdoutPath:      strings.TrimSpace(req.StdoutPath),
			StderrPath:      strings.TrimSpace(req.StderrPath),
			AppendOutput:    req.AppendOutput,
			ResumeSessionID: strings.TrimSpace(req.ResumeSession),
		})
		recordRemoteOperationMetric(remoteOpCodeAgentRun, startedAt, runErr)
		if runErr != nil {
			emitRemoteAuditEvent(requestID, "remote_codeagent_run", hostID+":"+agentID, "failure", map[string]interface{}{
				"backend":    backend,
				"capability": capability,
				"error":      runErr.Error(),
			})
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_CODEAGENT_RUN_FAILED", runErr.Error()))
			return
		}
		resultState := "success"
		if out.PolicyDecision == contract.PolicyDecisionDeny || out.PolicyDecision == contract.PolicyDecisionAsk {
			resultState = "blocked"
		}
		emitRemoteAuditEvent(requestID, "remote_codeagent_run", hostID+":"+agentID, resultState, map[string]interface{}{
			"backend":         backend,
			"capability":      capability,
			"policyDecision":  out.PolicyDecision,
			"costEstimateUsd": out.CostEstimateUSD,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"run": map[string]interface{}{
				"backend": backend,
				"result":  out,
			},
		})
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported codeagent action"))
		return
	}
}

func parseCodeAgentCapability(raw string) (contract.Capability, error) {
	capability := contract.Capability(strings.TrimSpace(raw))
	switch capability {
	case contract.CapabilityReadFile,
		contract.CapabilityWriteFile,
		contract.CapabilityApplyPatch,
		contract.CapabilityRunShell,
		contract.CapabilityRunShellRedirect:
		return capability, nil
	default:
		return "", fmt.Errorf("capability must be one of read_file, write_file, apply_patch, run_shell, run_shell_redirect")
	}
}

func resolveCodeAgentBackend(raw string) string {
	backend := strings.ToLower(strings.TrimSpace(raw))
	if backend == "" {
		return "codex"
	}
	switch backend {
	case "codex", "opencode":
		return backend
	default:
		return "codex"
	}
}

func normalizeWorkspaceRoot(raw string) string {
	workspace := strings.TrimSpace(raw)
	if workspace == "" {
		return "/workspace"
	}
	return workspace
}

func newRemoteCodeAgentAdapter(host RemoteHost, backend string) (contract.Adapter, error) {
	switch resolveCodeAgentBackend(backend) {
	case "codex":
		return codex.NewAdapter(codex.Options{
			MaxRetries: 1,
			Runner: func(ctx context.Context, command string, args []string) (codex.RunResult, error) {
				res, err := runRemoteCodeAgentCommand(ctx, host, command, args)
				return codex.RunResult{
					Stdout:   res.Stdout,
					Stderr:   res.Stderr,
					ExitCode: res.ExitCode,
				}, err
			},
		}), nil
	case "opencode":
		return opencode.NewAdapter(opencode.Options{
			MaxRetries: 1,
			Runner: func(ctx context.Context, command string, args []string) (opencode.RunResult, error) {
				res, err := runRemoteCodeAgentCommand(ctx, host, command, args)
				return opencode.RunResult{
					Stdout:   res.Stdout,
					Stderr:   res.Stderr,
					ExitCode: res.ExitCode,
				}, err
			},
		}), nil
	default:
		return nil, fmt.Errorf("unsupported backend %q", backend)
	}
}

func runRemoteCodeAgentCommand(ctx context.Context, host RemoteHost, command string, args []string) (remoteExecResult, error) {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellSingleQuote(strings.TrimSpace(command)))
	for _, arg := range args {
		parts = append(parts, shellSingleQuote(arg))
	}
	shellCommand := strings.Join(parts, " ")
	return runRemoteCommandWithRetry(ctx, host, shellCommand, 1)
}

func remoteInstallCodeAgentBinary(ctx context.Context, host RemoteHost, backend string) error {
	backend = resolveCodeAgentBackend(backend)
	plan, err := buildRemoteCodeAgentInstallPlan(ctx, host, backend)
	if err != nil {
		return err
	}
	if plan.AlreadyInstalled {
		return nil
	}
	res, err := runRemoteCommandWithRetry(ctx, host, plan.InstallCommand, 1)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return remoteCommandError(res, "install "+backend)
	}
	verifyRes, verifyErr := runRemoteCommandWithRetry(ctx, host, "command -v "+plan.BinaryName+" >/dev/null 2>&1", 1)
	if verifyErr != nil {
		return verifyErr
	}
	if verifyRes.ExitCode != 0 {
		return fmt.Errorf("%s install completed but binary %q is still missing from PATH", backend, plan.BinaryName)
	}
	return nil
}

type remoteCodeAgentInstallPlan struct {
	Backend          string
	BinaryName       string
	AlreadyInstalled bool
	Installer        string
	InstallCommand   string
}

func buildRemoteCodeAgentInstallPlan(ctx context.Context, host RemoteHost, backend string) (remoteCodeAgentInstallPlan, error) {
	normalized := resolveCodeAgentBackend(backend)
	plan := remoteCodeAgentInstallPlan{Backend: normalized}
	switch normalized {
	case "codex":
		plan.BinaryName = "codex"
	case "opencode":
		plan.BinaryName = "opencode"
	default:
		return remoteCodeAgentInstallPlan{}, fmt.Errorf("unsupported backend %q", backend)
	}

	hasBinary, err := remoteCommandExists(ctx, host, plan.BinaryName)
	if err != nil {
		return remoteCodeAgentInstallPlan{}, err
	}
	if hasBinary {
		plan.AlreadyInstalled = true
		return plan, nil
	}

	if ok, checkErr := remoteCommandExists(ctx, host, "bun"); checkErr == nil && ok {
		plan.Installer = "bun"
	} else {
		okNPM, npmErr := remoteCommandExists(ctx, host, "npm")
		if npmErr != nil {
			return remoteCodeAgentInstallPlan{}, npmErr
		}
		if okNPM {
			plan.Installer = "npm"
		}
	}
	if plan.Installer == "" {
		return remoteCodeAgentInstallPlan{}, fmt.Errorf("cannot install %s: neither bun nor npm is available on remote host", normalized)
	}

	switch normalized {
	case "codex":
		if plan.Installer == "bun" {
			plan.InstallCommand = "bun add -g @openai/codex >/dev/null 2>&1"
		} else {
			plan.InstallCommand = "npm install -g @openai/codex >/dev/null 2>&1"
		}
	case "opencode":
		if plan.Installer == "bun" {
			plan.InstallCommand = "bun add -g opencode-ai >/dev/null 2>&1"
		} else {
			plan.InstallCommand = "npm install -g opencode-ai >/dev/null 2>&1"
		}
	default:
		return remoteCodeAgentInstallPlan{}, fmt.Errorf("unsupported backend %q", backend)
	}
	return plan, nil
}

func remoteCommandExists(ctx context.Context, host RemoteHost, command string) (bool, error) {
	res, err := runRemoteCommandWithRetry(ctx, host, "command -v "+strings.TrimSpace(command)+" >/dev/null 2>&1", 1)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}
