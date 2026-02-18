//go:build !windows
// +build !windows

package lifecycle

import (
	"os/exec"
	"syscall"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for clean signal handling
	}
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	return syscall.Kill(-pid, signal)
}

// Platform signal constants.
var (
	sigTERM    = syscall.SIGTERM
	sigKILL    = syscall.SIGKILL
	signalZero = syscall.Signal(0)
)

// isNoSuchProcess returns true if the error indicates the process no longer exists.
func isNoSuchProcess(err error) bool {
	return err == syscall.ESRCH
}
