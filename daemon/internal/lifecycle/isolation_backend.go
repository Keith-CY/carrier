package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"carrier/daemon/internal/manifest"
)

const (
	defaultLimaInstanceEnvKey = "CARRIER_ISOLATION_LIMA_INSTANCE"
	defaultLimaInstanceName   = "default"
	isolationWorkDirEnvKey    = "CARRIER_ISOLATION_WORKDIR"
	defaultWSLDistroEnvKey    = "CARRIER_ISOLATION_WSL_DISTRO"
)

var (
	isolationRuntimeGOOS        = runtime.GOOS
	isolationBackendLookup      = exec.LookPath
	isolationEnvLookup          = os.Getenv
	isolationLimaPathCandidates = []string{"/opt/homebrew/bin/limactl", "/usr/local/bin/limactl"}
	isolationPathStat           = os.Stat
)

func buildIsolationHostPrepareCommand() (string, error) {
	switch strings.ToLower(strings.TrimSpace(isolationRuntimeGOOS)) {
	case manifest.CommandOSLinux:
		return buildHostEnsureLinuxIsolationDepsCommand(), nil
	case manifest.CommandOSDarwin:
		return buildHostEnsureDarwinIsolationDepsCommand(), nil
	case manifest.CommandOSWindows:
		if _, err := isolationBackendLookup("wsl"); err != nil {
			return "", fmt.Errorf("%w: WSL executable (wsl) not found in PATH; run `wsl --install` and reboot, then retry", ErrIsolationUnavailable)
		}
		return "", nil
	default:
		return "", fmt.Errorf("%w: unsupported host OS %s", ErrIsolationUnavailable, isolationRuntimeGOOS)
	}
}

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
	limactlPath  string
	instance     string
	agentWorkDir string
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

func (b limaIsolationBackend) generateLimaTemplate(instanceName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	templateDir := filepath.Join(homeDir, ".carrier", "lima")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		return "", fmt.Errorf("create lima template directory: %w", err)
	}

	templatePath := filepath.Join(templateDir, instanceName+".yaml")
	var templateContent string
	if strings.TrimSpace(b.agentWorkDir) == "" {
		templateContent = strings.TrimSpace(`
mounts: []
provision:
  - mode: system
    script: |
      apt-get update -qq && apt-get install -y -qq bubblewrap git curl
`) + "\n"
	} else {
		templateContent = fmt.Sprintf(strings.TrimSpace(`
mounts:
  - location: "%s"
    writable: true
provision:
  - mode: system
    script: |
      apt-get update -qq && apt-get install -y -qq bubblewrap git curl
`)+"\n", strings.TrimSpace(b.agentWorkDir))
	}

	if err := os.WriteFile(templatePath, []byte(templateContent), 0o600); err != nil {
		return "", fmt.Errorf("write lima template: %w", err)
	}
	return templatePath, nil
}

func (b limaIsolationBackend) PrepareCommands() ([]string, error) {
	safeLimaPath := shellSingleQuote(strings.TrimSpace(b.limactlPath))
	safeInstance := shellSingleQuote(strings.TrimSpace(b.instance))
	templatePath, err := b.generateLimaTemplate(strings.TrimSpace(b.instance))
	if err != nil {
		return nil, err
	}
	safeTemplatePath := shellSingleQuote(strings.TrimSpace(templatePath))
	ensureInstance := fmt.Sprintf(
		"%s list 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -Fxq %s || %s create -y --name %s %s",
		safeLimaPath,
		safeInstance,
		safeLimaPath,
		safeInstance,
		safeTemplatePath,
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

func resolveIsolationBackend(agentWorkDir string) (isolationBackend, error) {
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
			for _, candidate := range isolationLimaPathCandidates {
				info, statErr := isolationPathStat(candidate)
				if statErr == nil && !info.IsDir() {
					limactlPath = candidate
					break
				}
			}
		}
		if strings.TrimSpace(limactlPath) == "" {
			return nil, fmt.Errorf("%w: Lima executable (limactl) not found in PATH; install Lima (for example: brew install lima) and ensure limactl is available", ErrIsolationUnavailable)
		}
		instance := strings.TrimSpace(isolationEnvLookup(defaultLimaInstanceEnvKey))
		if instance == "" {
			instance = defaultLimaInstanceName
		}
		workDir := strings.TrimSpace(agentWorkDir)
		if workDir == "" {
			workDir = strings.TrimSpace(isolationEnvLookup(isolationWorkDirEnvKey))
		}
		if workDir == "" {
			workDir = IsolationWorkDir(instance)
		}
		return limaIsolationBackend{
			limactlPath:  strings.TrimSpace(limactlPath),
			instance:     instance,
			agentWorkDir: workDir,
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

func IsolationWorkDir(instance string) string {
	trimmed := strings.TrimSpace(instance)
	if trimmed == "" {
		trimmed = defaultLimaInstanceName
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".carrier", "instances", trimmed)
	}
	return filepath.Join(homeDir, ".carrier", "instances", trimmed)
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

func buildHostEnsureLinuxIsolationDepsCommand() string {
	return strings.TrimSpace(`
set -e
required_tools="bwrap git curl tar bash"
missing_tools=""
for tool in $required_tools; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    missing_tools="$missing_tools $tool"
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
  missing_tools="$missing_tools openssl"
fi

if [ -z "$missing_tools" ]; then
  exit 0
fi

packages=""
for tool in $missing_tools; do
  case "$tool" in
    bwrap) packages="$packages bubblewrap" ;;
    *) packages="$packages $tool" ;;
  esac
done

run_pkg_install() {
  if command -v sudo >/dev/null 2>&1; then
    sudo -n "$@"
  else
    "$@"
  fi
}

if command -v apt-get >/dev/null 2>&1; then
  run_pkg_install apt-get update
  run_pkg_install apt-get install -y $packages
elif command -v dnf >/dev/null 2>&1; then
  run_pkg_install dnf install -y $packages
elif command -v yum >/dev/null 2>&1; then
  run_pkg_install yum install -y $packages
elif command -v pacman >/dev/null 2>&1; then
  run_pkg_install pacman -Sy --noconfirm $packages
elif command -v zypper >/dev/null 2>&1; then
  run_pkg_install zypper --non-interactive install $packages
else
  echo "no supported package manager found to install isolation host dependencies:$missing_tools" >&2
  exit 127
fi

for tool in $required_tools; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required host dependency missing after install: $tool" >&2
    exit 127
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
  echo "required checksum tool missing after install (need one of: sha256sum, shasum, openssl)" >&2
  exit 127
fi
`)
}

func buildHostEnsureDarwinIsolationDepsCommand() string {
	return strings.TrimSpace(`
set -e
if command -v limactl >/dev/null 2>&1; then
  exit 0
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "Lima executable (limactl) not found in PATH; install Lima first (for example: brew install lima)" >&2
  exit 127
fi

brew install lima

if command -v limactl >/dev/null 2>&1; then
  exit 0
fi
if [ -x /opt/homebrew/bin/limactl ] || [ -x /usr/local/bin/limactl ]; then
  exit 0
fi
echo "limactl still unavailable after brew install lima" >&2
exit 127
`)
}

func buildGuestEnsureBwrapCommand() string {
	return strings.TrimSpace(`
set -e
required_tools="bwrap git curl tar bash"
missing_tools=""
for tool in $required_tools; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    missing_tools="$missing_tools $tool"
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
  missing_tools="$missing_tools openssl"
fi

if [ -z "$missing_tools" ]; then
  exit 0
fi

packages=""
for tool in $missing_tools; do
  case "$tool" in
    bwrap) packages="$packages bubblewrap" ;;
    *) packages="$packages $tool" ;;
  esac
done

run_pkg_install() {
  if command -v sudo >/dev/null 2>&1; then
    sudo -n "$@"
  else
    "$@"
  fi
}

if command -v apt-get >/dev/null 2>&1; then
  run_pkg_install apt-get update
  run_pkg_install apt-get install -y $packages
elif command -v dnf >/dev/null 2>&1; then
  run_pkg_install dnf install -y $packages
elif command -v yum >/dev/null 2>&1; then
  run_pkg_install yum install -y $packages
elif command -v pacman >/dev/null 2>&1; then
  run_pkg_install pacman -Sy --noconfirm $packages
elif command -v zypper >/dev/null 2>&1; then
  run_pkg_install zypper --non-interactive install $packages
else
  echo "no supported package manager found to install guest dependencies:$missing_tools" >&2
  exit 127
fi

for tool in $required_tools; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required guest dependency missing after install: $tool" >&2
    exit 127
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
  echo "required guest checksum tool missing after install (need one of: sha256sum, shasum, openssl)" >&2
  exit 127
fi
`)
}
