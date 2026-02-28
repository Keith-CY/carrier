package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/commandexec"
)

type countingTriager struct {
	calls int
}

func (c *countingTriager) Analyze(_ context.Context, _ baseagent.Evidence) (baseagent.TriageResult, error) {
	c.calls++
	return baseagent.TriageResult{}, nil
}

func TestInstallWithIsolationRunsDeterministicPreparePipelineForDarwin(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "darwin"
	isolationBackendLookup = func(name string) (string, error) {
		if name != "limactl" {
			t.Fatalf("lookup name = %q, want limactl", name)
		}
		return "/opt/homebrew/bin/limactl", nil
	}
	isolationEnvLookup = func(string) string { return "" }

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 28, 6, 0, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithDiagnoseDir(t.TempDir()),
	)

	manifest := sampleManifest()
	if err := svc.RegisterManifest(manifest); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.InstallWithOptions(context.Background(), manifest.ID, InstallOptions{Isolation: true}); err != nil {
		t.Fatalf("InstallWithOptions: %v", err)
	}

	joined := strings.Join(runner.calls, "\n")
	for _, want := range []string{
		"limactl",
		"list",
		"start",
		"shell",
		"sh -lc",
		"install-openclaw",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected deterministic pipeline call containing %q, calls=\n%s", want, joined)
		}
	}
}

func TestInstallWithIsolationUsesLinuxInstallCommandOnDarwinAndWindows(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	cases := []struct {
		name           string
		hostGOOS       string
		lookupName     string
		lookupPath     string
		wrappedCommand string
	}{
		{
			name:           "darwin",
			hostGOOS:       "darwin",
			lookupName:     "limactl",
			lookupPath:     "/opt/homebrew/bin/limactl",
			wrappedCommand: "limactl",
		},
		{
			name:           "windows",
			hostGOOS:       "windows",
			lookupName:     "wsl",
			lookupPath:     "/usr/bin/wsl",
			wrappedCommand: "wsl",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolationRuntimeGOOS = tc.hostGOOS
			isolationBackendLookup = func(name string) (string, error) {
				if name != tc.lookupName {
					t.Fatalf("lookup name = %q, want %q", name, tc.lookupName)
				}
				return tc.lookupPath, nil
			}
			isolationEnvLookup = func(string) string { return "" }

			runner := &fakeRunner{}
			checker := &fakeChecker{}
			clock := &fakeClock{current: time.Date(2026, 2, 28, 6, 8, 0, 0, time.UTC)}
			svc := NewService(nil,
				WithRunner(runner),
				WithRuntimeChecker(checker),
				WithNow(clock.Now),
				WithDiagnoseDir(t.TempDir()),
			)

			m := sampleManifest()
			m.ID = "picoclaw"
			m.Name = "PicoClaw"
			m.Runtime.Install.Command = "install-default"
			m.Runtime.Install.CommandByOS = map[string]string{
				"linux":   "install-linux",
				"darwin":  "install-darwin",
				"windows": "install-windows",
			}
			m.Runtime.Upgrade.Command = m.Runtime.Install.Command
			m.Runtime.Upgrade.CommandByOS = m.Runtime.Install.CommandByOS
			if err := svc.RegisterManifest(m); err != nil {
				t.Fatalf("RegisterManifest: %v", err)
			}
			if err := svc.InstallWithOptions(context.Background(), m.ID, InstallOptions{Isolation: true}); err != nil {
				t.Fatalf("InstallWithOptions: %v", err)
			}

			joined := strings.Join(runner.calls, "\n")
			if !strings.Contains(joined, "install-linux") {
				t.Fatalf("expected linux install command under isolation, calls=\n%s", joined)
			}
			if strings.Contains(joined, "install-darwin") || strings.Contains(joined, "install-windows") {
				t.Fatalf("unexpected host-specific install command under isolation, calls=\n%s", joined)
			}
			if !strings.Contains(joined, tc.wrappedCommand) {
				t.Fatalf("expected wrapped host runtime command %q, calls=\n%s", tc.wrappedCommand, joined)
			}
		})
	}
}

func TestInstallWithIsolationSkipsBaseAgentFallbackForNonOpenClaw(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) { return "/usr/bin/bwrap", nil }
	isolationEnvLookup = func(string) string { return "" }
	cases := []struct {
		id      string
		name    string
		command string
	}{
		{id: "picoclaw", name: "PicoClaw", command: "install-picoclaw"},
		{id: "zeroclaw", name: "ZeroClaw", command: "install-zeroclaw"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			runner := &fakeRunner{
				onRun: func(command string, _ int) (runResult, bool) {
					if strings.Contains(command, tc.command) {
						return runResult{
							result: commandexec.Result{ExitCode: 1, CombinedOutput: "install failed"},
							err:    errors.New("install failed"),
						}, true
					}
					return runResult{}, false
				},
			}
			checker := &fakeChecker{}
			clock := &fakeClock{current: time.Date(2026, 2, 28, 6, 10, 0, 0, time.UTC)}
			triager := &countingTriager{}
			svc := NewService(triager,
				WithRunner(runner),
				WithRuntimeChecker(checker),
				WithNow(clock.Now),
				WithDiagnoseDir(t.TempDir()),
			)

			manifest := sampleManifest()
			manifest.ID = tc.id
			manifest.Name = tc.name
			manifest.Runtime.Install.Command = tc.command
			manifest.Runtime.Upgrade.Command = tc.command
			if err := svc.RegisterManifest(manifest); err != nil {
				t.Fatalf("RegisterManifest: %v", err)
			}

			err := svc.InstallWithOptions(context.Background(), manifest.ID, InstallOptions{Isolation: true})
			if err == nil {
				t.Fatal("expected install failure")
			}
			if triager.calls != 0 {
				t.Fatalf("expected baseagent triage not to be called for non-openclaw isolation install, got %d", triager.calls)
			}
		})
	}
}

func TestInstallWithIsolationFallsBackToBaseAgentForOpenClawAfterDeterministicFailure(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) { return "/usr/bin/bwrap", nil }
	isolationEnvLookup = func(string) string { return "" }

	runner := &fakeRunner{
		onRun: func(command string, _ int) (runResult, bool) {
			if strings.Contains(command, "install-openclaw") {
				return runResult{
					result: commandexec.Result{ExitCode: 1, CombinedOutput: "install failed"},
					err:    errors.New("install failed"),
				}, true
			}
			return runResult{}, false
		},
	}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 28, 6, 20, 0, 0, time.UTC)}
	triager := &countingTriager{}
	svc := NewService(triager,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithDiagnoseDir(t.TempDir()),
	)

	manifest := sampleManifest()
	if err := svc.RegisterManifest(manifest); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}

	err := svc.InstallWithOptions(context.Background(), manifest.ID, InstallOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected install failure")
	}
	if triager.calls == 0 {
		t.Fatal("expected baseagent triage fallback to be invoked for openclaw")
	}
}
