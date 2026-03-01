package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const instanceStoreEnvKey = "CARRIER_INSTANCE_STORE"

var (
	validLimaInstanceName           = regexp.MustCompile(`^carrier-[a-zA-Z0-9][a-zA-Z0-9._-]*-[0-9a-f]{4,16}$`)
	isolationUserHomeDir            = os.UserHomeDir
	isolationRandReader   io.Reader = rand.Reader
	isolationEvalSymlinks           = filepath.EvalSymlinks
)

type limaTemplate struct {
	Images    []limaImage     `yaml:"images,omitempty"`
	Mounts    []limaMount     `yaml:"mounts"`
	MountType string          `yaml:"mountType,omitempty"`
	Provision []limaProvision `yaml:"provision,omitempty"`
	CPUs      int             `yaml:"cpus,omitempty"`
	Memory    string          `yaml:"memory,omitempty"`
	Disk      string          `yaml:"disk,omitempty"`
	Networks  []string        `yaml:"networks,omitempty"`
}

type limaImage struct {
	Location string `yaml:"location"`
}

type limaMount struct {
	Location string `yaml:"location"`
	Writable bool   `yaml:"writable"`
}

type limaProvision struct {
	Mode   string `yaml:"mode,omitempty"`
	Script string `yaml:"script,omitempty"`
}

func validateLimaInstanceName(name string) error {
	trimmed := strings.TrimSpace(name)
	if !validLimaInstanceName.MatchString(trimmed) {
		return fmt.Errorf("%w: invalid lima instance name %q", ErrIsolationUnavailable, name)
	}
	return nil
}

func sanitizeForInstanceName(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}

// sanitizeForWorkspacePath is less restrictive than sanitizeForInstanceName.
// It preserves case and allows characters valid in filesystem paths to prevent
// collisions between different agent IDs (e.g., agent1, Agent1, agent.1, agent_1).
func sanitizeForWorkspacePath(agentID string) string {
	trimmed := strings.TrimSpace(agentID)
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			// Encode other characters as hex to preserve uniqueness
			b.WriteString(fmt.Sprintf("-%02x-", r))
		}
	}
	result := b.String()
	// Remove leading/trailing dashes from hex encoding artifacts
	return strings.Trim(result, "-")
}

func generateLimaInstanceName(agentID string) (string, error) {
	sanitized := sanitizeForInstanceName(agentID)
	if sanitized == "" {
		return "", fmt.Errorf("%w: agent ID %q is not valid for lima instance generation", ErrIsolationUnavailable, agentID)
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(isolationRandReader, buf); err != nil {
		return "", fmt.Errorf("%w: generate lima instance suffix: %v", ErrIsolationUnavailable, err)
	}
	name := fmt.Sprintf("carrier-%s-%s", sanitized, hex.EncodeToString(buf))
	if err := validateLimaInstanceName(name); err != nil {
		return "", err
	}
	return name, nil
}

func resolveAllowedWorkspacePrefix() (string, error) {
	var prefix string
	if custom := strings.TrimSpace(isolationEnvLookup(instanceStoreEnvKey)); custom != "" {
		storePath, err := filepath.Abs(custom)
		if err != nil {
			return "", fmt.Errorf("%w: resolve instance store path %q: %v", ErrIsolationUnavailable, custom, err)
		}
		prefix = filepath.Clean(filepath.Dir(storePath))
	} else {
		home, err := isolationUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%w: resolve user home dir: %v", ErrIsolationUnavailable, err)
		}
		prefix = filepath.Join(home, ".carrier", "instances")
	}
	// Resolve symlinks to handle macOS /var -> /private/var case.
	resolved, err := isolationEvalSymlinks(prefix)
	if err != nil {
		// If the prefix doesn't exist yet, return the cleaned path.
		if os.IsNotExist(err) {
			return prefix, nil
		}
		return "", fmt.Errorf("%w: resolve prefix path %q: %v", ErrIsolationUnavailable, prefix, err)
	}
	return resolved, nil
}

func resolveWorkspacePathForAgent(agentID string) (string, error) {
	prefix, err := resolveAllowedWorkspacePrefix()
	if err != nil {
		return "", err
	}
	// Use sanitizeForWorkspacePath (not sanitizeForInstanceName) to preserve uniqueness
	// across different agent IDs with similar characters (e.g., agent1 vs Agent1 vs agent.1).
	sanitized := sanitizeForWorkspacePath(agentID)
	if sanitized == "" {
		return "", fmt.Errorf("%w: agent ID %q is not valid for workspace resolution", ErrIsolationUnavailable, agentID)
	}
	path := filepath.Join(prefix, sanitized, "workspace")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("%w: create workspace path %q: %v", ErrIsolationUnavailable, path, err)
	}
	return path, nil
}

func validateWorkspacePath(workspacePath string) error {
	cleaned := filepath.Clean(strings.TrimSpace(workspacePath))
	if cleaned == "" || cleaned == "." {
		return fmt.Errorf("%w: workspace path is empty", ErrIsolationUnavailable)
	}
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("%w: workspace path must be absolute: %q", ErrIsolationUnavailable, workspacePath)
	}
	resolved, err := isolationEvalSymlinks(cleaned)
	if err != nil {
		return fmt.Errorf("%w: resolve workspace path %q: %v", ErrIsolationUnavailable, cleaned, err)
	}
	prefix, err := resolveAllowedWorkspacePrefix()
	if err != nil {
		return err
	}
	if !isPathWithinBase(prefix, resolved) {
		return fmt.Errorf("%w: workspace path %q is outside allowed prefix %q", ErrIsolationUnavailable, resolved, prefix)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("%w: stat workspace path %q: %v", ErrIsolationUnavailable, resolved, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: workspace path is not a directory: %q", ErrIsolationUnavailable, resolved)
	}
	return nil
}

func limaTemplateDir() (string, error) {
	home, err := isolationUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: resolve user home dir: %v", ErrIsolationUnavailable, err)
	}
	return filepath.Join(home, ".carrier", "lima", "templates"), nil
}

func templatePath(instanceName string) (string, error) {
	if err := validateLimaInstanceName(instanceName); err != nil {
		return "", err
	}
	root, err := limaTemplateDir()
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	path := filepath.Join(root, strings.TrimSpace(instanceName)+".yaml")
	if !isPathWithinBase(root, path) {
		return "", fmt.Errorf("%w: template path escapes root: %q", ErrIsolationUnavailable, path)
	}
	return path, nil
}

func isPathWithinBase(base, target string) bool {
	baseClean := filepath.Clean(base)
	targetClean := filepath.Clean(target)
	rel, err := filepath.Rel(baseClean, targetClean)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func generateLimaTemplate(instanceName, workspacePath string) ([]byte, error) {
	if err := validateLimaInstanceName(instanceName); err != nil {
		return nil, err
	}
	if err := validateWorkspacePath(workspacePath); err != nil {
		return nil, err
	}
	resolvedPath, err := isolationEvalSymlinks(filepath.Clean(workspacePath))
	if err != nil {
		return nil, fmt.Errorf("%w: resolve workspace path for template: %v", ErrIsolationUnavailable, err)
	}
	template := limaTemplate{
		Images: []limaImage{
			{Location: "ubuntu-lts"},
		},
		Mounts: []limaMount{
			{Location: resolvedPath, Writable: true},
		},
		MountType: "reverse-sshfs",
		Provision: []limaProvision{
			{
				Mode: "system",
				Script: strings.TrimSpace(`
#!/bin/bash
set -eu
apt-get update -qq
apt-get install -y -qq bubblewrap git curl tar bash
`),
			},
		},
		CPUs:   2,
		Memory: "2GiB",
		Disk:   "10GiB",
	}
	body, err := yaml.Marshal(template)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal lima template: %v", ErrIsolationUnavailable, err)
	}
	header := []byte("# Managed by Carrier - do not edit manually\n")
	return append(header, body...), nil
}

func writeLimaTemplate(instanceName, workspacePath string) (string, error) {
	path, err := templatePath(instanceName)
	if err != nil {
		return "", err
	}
	content, err := generateLimaTemplate(instanceName, workspacePath)
	if err != nil {
		return "", err
	}
	root := filepath.Dir(path)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("%w: create template directory %q: %v", ErrIsolationUnavailable, root, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", fmt.Errorf("%w: write lima template %q: %v", ErrIsolationUnavailable, path, err)
	}
	return path, nil
}

func removeLimaTemplate(instanceName string) error {
	path, err := templatePath(instanceName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: remove lima template %q: %v", ErrIsolationUnavailable, path, err)
	}
	return nil
}
