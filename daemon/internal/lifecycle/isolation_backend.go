package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"carrier/daemon/internal/manifest"
)

const (
	defaultLimaInstanceEnvKey = "CARRIER_ISOLATION_LIMA_INSTANCE"
	defaultLimaInstanceName   = "default"
	defaultWSLDistroEnvKey    = "CARRIER_ISOLATION_WSL_DISTRO"
)

var (
	isolationRuntimeGOOS   = runtime.GOOS
	isolationBackendLookup = exec.LookPath
	isolationEnvLookup     = os.Getenv
)

type isolationBackend interface {
	CommandGOOS() string
	WrapCommand(command string) (string, error)
	WrapStartCommand(startCommand string) (string, error)
	PrepareCommands() ([]string, error)
}

type linuxIsolationBackend struct {
	bwrapPath string
}

func (b linuxIsolationBackend) CommandGOOS() string {
	return manifest.CommandOSLinux
}

func (b linuxIsolationBackend) WrapCommand(command string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("%w: runtime command is empty", ErrIsolationUnavailable)
	}
	return trimmed, nil
}

func (b linuxIsolationBackend) WrapStartCommand(startCommand string) (string, error) {
	return buildBwrapInvocation(b.bwrapPath, startCommand)
}

func (b linuxIsolationBackend) PrepareCommands() ([]string, error) {
	return []string{
		fmt.Sprintf("command -v %s >/dev/null 2>&1", shellSingleQuote(strings.TrimSpace(b.bwrapPath))),
	}, nil
}

type limaIsolationBackend struct {
	limactlPath string
	instance    string
}

func (b limaIsolationBackend) CommandGOOS() string {
	return manifest.CommandOSLinux
}

func (b limaIsolationBackend) WrapCommand(command string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("%w: runtime command is empty", ErrIsolationUnavailable)
	}
	safeLimaPath := shellSingleQuote(strings.TrimSpace(b.limactlPath))
	safeInstance := shellSingleQuote(strings.TrimSpace(b.instance))
	safeCommand := shellSingleQuote(trimmed)
	return fmt.Sprintf("%s shell %s -- sh -lc %s", safeLimaPath, safeInstance, safeCommand), nil
}

func (b limaIsolationBackend) WrapStartCommand(startCommand string) (string, error) {
	guestCommand, err := buildGuestBwrapCommand(startCommand)
	if err != nil {
		return "", err
	}
	return b.WrapCommand(guestCommand)
}

func (b limaIsolationBackend) PrepareCommands() ([]string, error) {
	safeLimaPath := shellSingleQuote(strings.TrimSpace(b.limactlPath))
	safeInstance := shellSingleQuote(strings.TrimSpace(b.instance))
	ensureInstance := fmt.Sprintf(
		"%s list 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -Fxq %s || %s create -y --name %s",
		safeLimaPath,
		safeInstance,
		safeLimaPath,
		safeInstance,
	)
	startInstance := fmt.Sprintf("%s start %s", safeLimaPath, safeInstance)
	ensureGuestBwrap, err := b.WrapCommand(buildGuestEnsureBwrapCommand())
	if err != nil {
		return nil, err
	}
	return []string{ensureInstance, startInstance, ensureGuestBwrap}, nil
}

type wslIsolationBackend struct {
	wslPath string
	distro  string
}

func (b wslIsolationBackend) CommandGOOS() string {
	return manifest.CommandOSLinux
}

func (b wslIsolationBackend) WrapCommand(command string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("%w: runtime command is empty", ErrIsolationUnavailable)
	}
	safeWSLPath := shellSingleQuote(strings.TrimSpace(b.wslPath))
	safeCommand := shellSingleQuote(trimmed)
	distro := strings.TrimSpace(b.distro)
	if distro != "" {
		return fmt.Sprintf("%s -d %s -- sh -lc %s", safeWSLPath, shellSingleQuote(distro), safeCommand), nil
	}
	return fmt.Sprintf("%s -- sh -lc %s", safeWSLPath, safeCommand), nil
}

func (b wslIsolationBackend) WrapStartCommand(startCommand string) (string, error) {
	guestCommand, err := buildGuestBwrapCommand(startCommand)
	if err != nil {
		return "", err
	}
	return b.WrapCommand(guestCommand)
}

func (b wslIsolationBackend) PrepareCommands() ([]string, error) {
	ensureGuestBwrap, err := b.WrapCommand(buildGuestEnsureBwrapCommand())
	if err != nil {
		return nil, err
	}
	return []string{ensureGuestBwrap}, nil
}

func resolveIsolationBackend() (isolationBackend, error) {
	switch strings.ToLower(strings.TrimSpace(isolationRuntimeGOOS)) {
	case manifest.CommandOSLinux:
		bwrapPath, err := isolationBackendLookup("bwrap")
		if err != nil || strings.TrimSpace(bwrapPath) == "" {
			return nil, fmt.Errorf("%w: bubblewrap (bwrap) executable not found in PATH", ErrIsolationUnavailable)
		}
		return linuxIsolationBackend{bwrapPath: strings.TrimSpace(bwrapPath)}, nil
	case manifest.CommandOSDarwin:
		limactlPath, err := isolationBackendLookup("limactl")
		if err != nil || strings.TrimSpace(limactlPath) == "" {
			return nil, fmt.Errorf("%w: Lima executable (limactl) not found in PATH; install Lima and ensure limactl is available", ErrIsolationUnavailable)
		}
		instance := strings.TrimSpace(isolationEnvLookup(defaultLimaInstanceEnvKey))
		if instance == "" {
			instance = defaultLimaInstanceName
		}
		return limaIsolationBackend{
			limactlPath: strings.TrimSpace(limactlPath),
			instance:    instance,
		}, nil
	case manifest.CommandOSWindows:
		wslPath, err := isolationBackendLookup("wsl")
		if err != nil || strings.TrimSpace(wslPath) == "" {
			return nil, fmt.Errorf("%w: WSL executable (wsl) not found in PATH; enable/install WSL2 first", ErrIsolationUnavailable)
		}
		return wslIsolationBackend{
			wslPath: strings.TrimSpace(wslPath),
			distro:  strings.TrimSpace(isolationEnvLookup(defaultWSLDistroEnvKey)),
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported host OS %s", ErrIsolationUnavailable, isolationRuntimeGOOS)
	}
}

func buildIsolationStartCommand(startCommand string) (string, error) {
	backend, err := resolveIsolationBackend()
	if err != nil {
		return "", err
	}
	return backend.WrapStartCommand(startCommand)
}

func buildBwrapInvocation(bwrapExecutable, startCommand string) (string, error) {
	trimmedStartCommand := strings.TrimSpace(startCommand)
	if trimmedStartCommand == "" {
		return "", fmt.Errorf("%w: runtime start command is empty", ErrIsolationUnavailable)
	}
	trimmedBwrapExecutable := strings.TrimSpace(bwrapExecutable)
	if trimmedBwrapExecutable == "" {
		return "", fmt.Errorf("%w: bubblewrap (bwrap) executable not found", ErrIsolationUnavailable)
	}
	safeBwrapPath := shellSingleQuote(trimmedBwrapExecutable)
	safeStartCommand := shellSingleQuote(trimmedStartCommand)
	return fmt.Sprintf(
		"%s --die-with-parent --new-session --bind / / --proc /proc --dev /dev --tmpfs /tmp --unshare-pid -- sh -lc %s",
		safeBwrapPath,
		safeStartCommand,
	), nil
}

func buildGuestBwrapCommand(startCommand string) (string, error) {
	bwrapInvocation, err := buildBwrapInvocation("bwrap", startCommand)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"set -e; if ! command -v bwrap >/dev/null 2>&1; then echo 'bubblewrap (bwrap) executable not found in guest PATH' >&2; exit 127; fi; exec %s",
		bwrapInvocation,
	), nil
}

func buildGuestEnsureBwrapCommand() string {
	return strings.TrimSpace(`
set -e
if command -v bwrap >/dev/null 2>&1; then
  exit 0
fi

if command -v apt-get >/dev/null 2>&1; then
  sudo -n apt-get update
  sudo -n apt-get install -y bubblewrap
elif command -v dnf >/dev/null 2>&1; then
  sudo -n dnf install -y bubblewrap
elif command -v yum >/dev/null 2>&1; then
  sudo -n yum install -y bubblewrap
elif command -v pacman >/dev/null 2>&1; then
  sudo -n pacman -Sy --noconfirm bubblewrap
elif command -v zypper >/dev/null 2>&1; then
  sudo -n zypper --non-interactive install bubblewrap
else
  echo "no supported package manager found to install bubblewrap" >&2
  exit 127
fi

if ! command -v bwrap >/dev/null 2>&1; then
  echo "bubblewrap (bwrap) installation did not produce executable in PATH" >&2
  exit 127
fi
`)
}
