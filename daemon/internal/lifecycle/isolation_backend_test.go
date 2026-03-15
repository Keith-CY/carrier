package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerAgentLimaPrepareCommandsUsesCustomTemplate(t *testing.T) {
	origHome := isolationUserHomeDir
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationUserHomeDir = origHome
		isolationEnvLookup = origEnv
	})

	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }
	isolationEnvLookup = func(string) string { return "" }

	workspace := filepath.Join(home, ".carrier", "instances", "openclaw", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	backend := &perAgentLimaIsolationBackend{
		limactlPath:   "/usr/local/bin/limactl",
		instanceName:  "carrier-openclaw-a3f2b1c4",
		workspacePath: workspace,
	}
	commands, err := backend.PrepareCommands()
	if err != nil {
		t.Fatalf("PrepareCommands: %v", err)
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "create -y --name 'carrier-openclaw-a3f2b1c4'") {
		t.Fatalf("expected create command with instance name, commands=\n%s", joined)
	}
	if !strings.Contains(joined, ".yaml") {
		t.Fatalf("expected template path in create command, commands=\n%s", joined)
	}
	if strings.Contains(joined, "--name 'default'") {
		t.Fatalf("unexpected default lima instance reuse, commands=\n%s", joined)
	}
}

func TestPerAgentLimaCleanupStopsAndDeletes(t *testing.T) {
	origHome := isolationUserHomeDir
	t.Cleanup(func() {
		isolationUserHomeDir = origHome
	})
	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }

	instance := "carrier-openclaw-a3f2b1c4"
	tmpl, err := writeLimaTemplate(instance, createWorkspaceForCleanupTest(t, home))
	if err != nil {
		t.Fatalf("writeLimaTemplate: %v", err)
	}
	if _, err := os.Stat(tmpl); err != nil {
		t.Fatalf("expected template file before cleanup: %v", err)
	}

	callsPath := filepath.Join(t.TempDir(), "calls.txt")
	limactlPath := filepath.Join(t.TempDir(), "limactl")
	script := "#!/bin/sh\n" +
		"echo \"$1 $2\" >> " + shellSingleQuote(callsPath) + "\n" +
		"exit 0\n"
	if err := os.WriteFile(limactlPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write limactl script: %v", err)
	}

	backend := &perAgentLimaIsolationBackend{
		limactlPath:  limactlPath,
		instanceName: instance,
	}
	if err := backend.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	contents, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	log := string(contents)
	if !strings.Contains(log, "stop "+instance) || !strings.Contains(log, "delete "+instance) {
		t.Fatalf("expected stop/delete calls, got %q", log)
	}
	if _, err := os.Stat(tmpl); !os.IsNotExist(err) {
		t.Fatalf("expected template to be removed, stat err=%v", err)
	}
}

func createWorkspaceForCleanupTest(t *testing.T, home string) string {
	t.Helper()
	workspace := filepath.Join(home, ".carrier", "instances", "openclaw", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	return workspace
}

func TestBuildHostEnsureLinuxIsolationDepsCommandRepairsUnusableBwrap(t *testing.T) {
	cmd := buildHostEnsureLinuxIsolationDepsCommand()
	for _, want := range []string{
		`bwrap --bind / / --proc /proc --dev /dev --tmpfs /tmp --unshare-pid -- sh -lc "exit 0"`,
		`run_pkg_install chmod u+s "$(command -v bwrap)"`,
		`bubblewrap is installed but unusable for isolation host setup`,
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("expected host isolation prep command to contain %q, got:\n%s", want, cmd)
		}
	}
}

func TestBuildBwrapInvocationPreservesTmpHomeRoot(t *testing.T) {
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationEnvLookup = origEnv
	})
	isolationEnvLookup = func(key string) string {
		if key == "HOME" {
			return "/tmp/clp.7q1sWL/h"
		}
		return ""
	}

	got, err := buildBwrapInvocation("/usr/bin/bwrap", "tail -f /dev/null")
	if err != nil {
		t.Fatalf("buildBwrapInvocation: %v", err)
	}
	for _, want := range []string{
		"--tmpfs /tmp",
		"--dir '/tmp/clp.7q1sWL'",
		"--bind '/tmp/clp.7q1sWL' '/tmp/clp.7q1sWL'",
		"tail -f /dev/null",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected wrapped invocation to contain %q, got %q", want, got)
		}
	}
}
