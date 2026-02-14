package catalog

import "carrier/daemon/internal/manifest"

const (
	openclawVersion = "1.0.0"
	openclawBaseURL = "https://github.com/openclaw/openclaw/releases/download/v1.0.0"
)

func OpenClawManifest() manifest.Manifest {
	// Pinned artifact URLs with explicit version - no dynamic script execution
	installCmd := `sh -c 'set -e; V="` + openclawVersion + `"; U="` + openclawBaseURL + `"; A="openclaw-$(uname -s)-$(uname -m).tar.gz"; curl -fsSL -o "$A" "$U/$A"; curl -fsSL -o "$A.sha256" "$U/$A.sha256"; sha256sum -c "$A.sha256"; tar xzf "$A"; install -m 755 openclaw /usr/local/bin/openclaw; rm -f "$A" "$A.sha256" openclaw'`
	
	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      openclawVersion,
		Description:  "Full-featured AI assistant with memory support",
		Capabilities: []string{"chat", "code", "memory"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: installCmd},
			Upgrade: manifest.CommandSpec{Command: installCmd},
			Start:   manifest.CommandSpec{Command: "openclaw gateway start"},
			Stop:    manifest.CommandSpec{Command: "openclaw gateway stop"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 8080}},
			Healthcheck: manifest.HealthcheckSpec{
				Type: "http",
				URL:  "http://localhost:8080/health",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{{Name: "OPENAI_API_KEY", Secret: true, Description: "OpenAI API key for LLM access"}},
			Optional: []manifest.EnvVar{{Name: "LOG_LEVEL", Default: "info", Description: "Logging verbosity level"}},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent, manifest.MemoryTypeShared, manifest.MemoryTypePublic},
			MountPath: "./memory",
		},
		Upgrade: manifest.UpgradeSpec{Channel: "stable", Strategy: "in_place_or_reinstall"},
		Health: manifest.HealthSpec{
			IntervalSeconds:   30,
			TimeoutSeconds:    5,
			Retries:           3,
			RestartLoopWindow: 300,
			RestartLoopMax:    5,
		},
		Diagnostics: manifest.Diagnostics{Include: []string{"runtime_logs", "process_state", "env_sanitized"}},
	}
}
