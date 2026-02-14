// Package commandexec defines the Runner interface for executing shell commands
// and provides a default ShellRunner implementation that delegates to the
// host's shell (/bin/sh on Unix, wsl.exe on Windows).
package commandexec
