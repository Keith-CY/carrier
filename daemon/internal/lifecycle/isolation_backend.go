package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"carrier/daemon/internal/manifest"
)

const (
	defaultWSLDistroEnvKey = "CARRIER_ISOLATION_WSL_DISTRO"
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
	Cleanup() error
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

func (b linuxIsolationBackend) Cleanup() error {
	return nil
}

type perAgentLimaIsolationBackend struct {
	limactlPath   string
	instanceName  string
	workspacePath string
	templatePath  string
}

func (b perAgentLimaIsolationBackend) CommandGOOS() string {
	return manifest.CommandOSLinux
}

func (b perAgentLimaIsolationBackend) WrapCommand(command string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("%w: runtime command is empty", ErrIsolationUnavailable)
	}
	if err := validateLimaInstanceName(b.instanceName); err != nil {
		return "", err
	}
	safeLimaPath := shellSingleQuote(strings.TrimSpace(b.limactlPath))
	safeInstance := shellSingleQuote(strings.TrimSpace(b.instanceName))
	safeCommand := shellSingleQuote(trimmed)
	return fmt.Sprintf("%s shell %s -- sh -lc %s", safeLimaPath, safeInstance, safeCommand), nil
}

func (b perAgentLimaIsolationBackend) WrapStartCommand(startCommand string) (string, error) {
	return b.WrapCommand(startCommand)
}

func (b *perAgentLimaIsolationBackend) ensureTemplatePath() (string, error) {
	if b.templatePath != "" {
		return b.templatePath, nil
	}
	if err := validateLimaInstanceName(b.instanceName); err != nil {
		return "", err
	}
	if _, err := validateWorkspacePath(b.workspacePath); err != nil {
		return "", err
	}
	path, err := writeLimaTemplate(b.instanceName, b.workspacePath)
	if err != nil {
		return "", err
	}
	b.templatePath = path
	return path, nil
}

func (b *perAgentLimaIsolationBackend) PrepareCommands() ([]string, error) {
	if err := validateLimaInstanceName(b.instanceName); err != nil {
		return nil, err
	}
	tmplPath, err := b.ensureTemplatePath()
	if err != nil {
		return nil, err
	}
	safeLimaPath := shellSingleQuote(strings.TrimSpace(b.limactlPath))
	safeInstance := shellSingleQuote(strings.TrimSpace(b.instanceName))
	safeTemplatePath := shellSingleQuote(strings.TrimSpace(tmplPath))
	ensureInstance := fmt.Sprintf(
		"%s list 2>/dev/null | tail -n +2 | awk '{print $1}' | grep -Fxq %s || %s create -y --name %s %s",
		safeLimaPath,
		safeInstance,
		safeLimaPath,
		safeInstance,
		safeTemplatePath,
	)
	startInstance := fmt.Sprintf("%s start %s", safeLimaPath, safeInstance)
	return []string{ensureInstance, startInstance}, nil
}

func (b *perAgentLimaIsolationBackend) Cleanup() error {
	if err := validateLimaInstanceName(b.instanceName); err != nil {
		return err
	}
	safePath := strings.TrimSpace(b.limactlPath)
	if safePath == "" {
		return fmt.Errorf("%w: lima executable (limactl) path is empty", ErrIsolationUnavailable)
	}

	var errs []error
	if out, err := exec.Command(safePath, "stop", b.instanceName).CombinedOutput(); err != nil {
		if !isLimaNotRunningError(err, string(out)) {
			errs = append(errs, fmt.Errorf("stop lima instance %q: %w (%s)", b.instanceName, err, strings.TrimSpace(string(out))))
		}
	}
	if out, err := exec.Command(safePath, "delete", b.instanceName).CombinedOutput(); err != nil {
		if !isLimaNotFoundError(err, string(out)) {
			errs = append(errs, fmt.Errorf("delete lima instance %q: %w (%s)", b.instanceName, err, strings.TrimSpace(string(out))))
		}
	}
	if err := removeLimaTemplate(b.instanceName); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func isLimaNotRunningError(err error, output string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error() + " " + output)
	return strings.Contains(message, "not running")
}

func isLimaNotFoundError(err error, output string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error() + " " + output)
	return strings.Contains(message, "not found") || strings.Contains(message, "does not exist")
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
	return b.WrapCommand(startCommand)
}

func (b wslIsolationBackend) PrepareCommands() ([]string, error) {
	return nil, nil
}

func (b wslIsolationBackend) Cleanup() error {
	return nil
}

type isolationBackendOptions struct {
	InstanceName  string
	WorkspacePath string
}

func resolveIsolationBackend(options isolationBackendOptions) (isolationBackend, error) {
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
		instance := strings.TrimSpace(options.InstanceName)
		if instance == "" {
			return nil, fmt.Errorf("%w: missing lima instance name for darwin isolation", ErrIsolationUnavailable)
		}
		if err := validateLimaInstanceName(instance); err != nil {
			return nil, err
		}
		return &perAgentLimaIsolationBackend{
			limactlPath:   strings.TrimSpace(limactlPath),
			instanceName:  instance,
			workspacePath: strings.TrimSpace(options.WorkspacePath),
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
	args := []string{
		safeBwrapPath,
		"--die-with-parent",
		"--new-session",
		"--bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}
	if tmpRoot := bwrapPreservedTmpRoot(strings.TrimSpace(isolationEnvLookup("HOME"))); tmpRoot != "" {
		safeTmpRoot := shellSingleQuote(tmpRoot)
		args = append(args,
			"--dir", safeTmpRoot,
			"--bind", safeTmpRoot, safeTmpRoot,
		)
	}
	args = append(args,
		"--unshare-pid",
		"--",
		"sh", "-lc", safeStartCommand,
	)
	return strings.Join(args, " "), nil
}

func bwrapPreservedTmpRoot(home string) string {
	trimmed := strings.TrimSpace(home)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/tmp/") {
		return ""
	}
	rest := strings.TrimPrefix(trimmed, "/tmp/")
	if rest == "" {
		return ""
	}
	segment, _, _ := strings.Cut(rest, "/")
	segment = strings.TrimSpace(segment)
	if segment == "" || segment == "." || segment == ".." {
		return ""
	}
	if strings.Contains(segment, "/") {
		return ""
	}
	return "/tmp/" + segment
}

func buildHostEnsureLinuxIsolationDepsCommand() string {
	return buildEnsureIsolationDepsScript("host")
}

func buildEnsureIsolationDepsScript(context string) string {
	label := strings.TrimSpace(context)
	if label == "" {
		label = "host"
	}
	return strings.TrimSpace(fmt.Sprintf(`
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

run_pkg_install() {
  if command -v sudo >/dev/null 2>&1; then
    sudo -n "$@"
  else
    "$@"
  fi
}

if [ -n "$missing_tools" ]; then
  packages=""
  for tool in $missing_tools; do
    case "$tool" in
      bwrap) packages="$packages bubblewrap" ;;
      *) packages="$packages $tool" ;;
    esac
  done

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
  elif command -v apk >/dev/null 2>&1; then
    run_pkg_install apk add $packages
  else
    echo "no supported package manager found to install isolation %s dependencies:$missing_tools" >&2
    exit 127
  fi
fi

for tool in $required_tools; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required %s dependency missing after install: $tool" >&2
    exit 127
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
  echo "required %s checksum tool missing after install (need one of: sha256sum, shasum, openssl)" >&2
  exit 127
fi

if command -v bwrap >/dev/null 2>&1; then
  if ! bwrap --bind / / --proc /proc --dev /dev --tmpfs /tmp --unshare-pid -- sh -lc "exit 0" >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1; then
      run_pkg_install chmod u+s "$(command -v bwrap)"
    fi
  fi
  if ! bwrap --bind / / --proc /proc --dev /dev --tmpfs /tmp --unshare-pid -- sh -lc "exit 0" >/dev/null 2>&1; then
    echo "bubblewrap is installed but unusable for isolation %s setup (need either unprivileged user namespaces or a setuid bwrap binary)" >&2
    exit 127
  fi
fi
`, label, label, label, label))
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
