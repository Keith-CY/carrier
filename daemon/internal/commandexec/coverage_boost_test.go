package commandexec

import (
	"runtime"
	"testing"
)

func TestNewShellRunnerUsesRuntimeGOOS(t *testing.T) {
	runner := NewShellRunner()
	if runner.GOOS != runtime.GOOS {
		t.Fatalf("GOOS=%q want=%q", runner.GOOS, runtime.GOOS)
	}
}
