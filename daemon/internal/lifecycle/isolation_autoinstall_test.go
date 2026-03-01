package lifecycle

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestAttemptAutoInstallBwrap(t *testing.T) {
	origLookPath := autoInstallLookPath
	origExecCmd := autoInstallExecCmd
	t.Cleanup(func() {
		autoInstallLookPath = origLookPath
		autoInstallExecCmd = origExecCmd
	})

	type cmdResult struct {
		code int
		out  string
	}
	cases := []struct {
		name          string
		availableBins map[string]bool
		commands      map[string]cmdResult
		wantErr       bool
		wantSudo      bool
	}{
		{
			name: "apt-get with sudo",
			availableBins: map[string]bool{
				"apt-get": true, "sudo": true, "bwrap": true,
			},
			commands: map[string]cmdResult{
				"sudo -n true":                          {code: 0},
				"sudo -n apt-get update":                {code: 0},
				"sudo -n apt-get install -y bubblewrap": {code: 0},
			},
			wantSudo: true,
		},
		{
			name: "sudo fails direct succeeds",
			availableBins: map[string]bool{
				"apt-get": true, "sudo": true, "bwrap": true,
			},
			commands: map[string]cmdResult{
				"sudo -n true":                          {code: 0},
				"sudo -n apt-get update":                {code: 0},
				"sudo -n apt-get install -y bubblewrap": {code: 1, out: "sudo denied"},
				"apt-get install -y bubblewrap":         {code: 0},
			},
			wantSudo: false,
		},
		{
			name: "no package manager",
			availableBins: map[string]bool{
				"sudo": true,
			},
			wantErr: true,
		},
		{
			name: "install succeeds but bwrap missing",
			availableBins: map[string]bool{
				"apt-get": true, "sudo": true,
			},
			commands: map[string]cmdResult{
				"sudo -n true":                          {code: 0},
				"sudo -n apt-get update":                {code: 0},
				"sudo -n apt-get install -y bubblewrap": {code: 0},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			autoInstallLookPath = func(name string) (string, error) {
				if tc.availableBins[name] {
					return "/usr/bin/" + name, nil
				}
				return "", errors.New("missing")
			}
			autoInstallExecCmd = func(name string, args ...string) *exec.Cmd {
				key := strings.TrimSpace(name + " " + strings.Join(args, " "))
				result, ok := tc.commands[key]
				if !ok {
					result = cmdResult{code: 1, out: "unexpected command: " + key}
				}
				return exec.Command("sh", "-c", scriptedCommand(result.out, result.code))
			}

			got, err := attemptAutoInstallBwrap()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("attemptAutoInstallBwrap() error = %v", err)
			}
			if got == nil || !got.Installed {
				t.Fatalf("expected installed result, got %#v", got)
			}
			if got.UsedSudo != tc.wantSudo {
				t.Fatalf("UsedSudo = %t, want %t", got.UsedSudo, tc.wantSudo)
			}
		})
	}
}

func scriptedCommand(output string, code int) string {
	escaped := strings.ReplaceAll(output, "'", "'\"'\"'")
	if strings.TrimSpace(escaped) == "" {
		return fmt.Sprintf("exit %d", code)
	}
	return fmt.Sprintf("printf '%s'; exit %d", escaped, code)
}
