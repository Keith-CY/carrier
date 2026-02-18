//go:build windows
// +build windows

package lifecycle

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(cmd *exec.Cmd) {
	_ = cmd
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(signal)
}

// Platform signal constants.
var (
	sigTERM    = syscall.SIGTERM
	sigKILL    = syscall.SIGKILL
	signalZero = syscall.Signal(0)
)

// isNoSuchProcess returns true if the error indicates the process no longer exists.
func isNoSuchProcess(err error) bool {
	// On Windows, os.FindProcess never fails and Signal returns os.ErrProcessDone
	// or a windows-specific error. Check for the common case.
	return err == os.ErrProcessDone
}
