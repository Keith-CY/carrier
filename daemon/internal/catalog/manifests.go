package catalog

import "carrier/daemon/internal/manifest"

func OpenClawManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      "1.0.0",
		Description:  "Full-featured AI assistant with memory support",
		Capabilities: []string{"chat", "code", "memory"},
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: `bash -c 'command -v npm >/dev/null 2>&1 && npm install -g openclaw@1.0.0 || (ARCHIVE="openclaw-$(uname -s)-$(uname -m).tar.gz" && curl -fsSL -o "$ARCHIVE" "https://github.com/openclaw/openclaw/releases/download/v1.0.0/$ARCHIVE" && curl -fsSL -o "$ARCHIVE.sha256" "https://github.com/openclaw/openclaw/releases/download/v1.0.0/$ARCHIVE.sha256" && sha256sum -c "$ARCHIVE.sha256" && tar xzf "$ARCHIVE" && install -m 755 openclaw /usr/local/bin/openclaw && rm -f "$ARCHIVE" "$ARCHIVE.sha256")'`},
			Upgrade: manifest.CommandSpec{Command: `bash -c 'command -v npm >/dev/null 2>&1 && npm install -g openclaw@1.0.0 || (ARCHIVE="openclaw-$(uname -s)-$(uname -m).tar.gz" && curl -fsSL -o "$ARCHIVE" "https://github.com/openclaw/openclaw/releases/download/v1.0.0/$ARCHIVE" && curl -fsSL -o "$ARCHIVE.sha256" "https://github.com/openclaw/openclaw/releases/download/v1.0.0/$ARCHIVE.sha256" && sha256sum -c "$ARCHIVE.sha256" && tar xzf "$ARCHIVE" && install -m 755 openclaw /usr/local/bin/openclaw && rm -f "$ARCHIVE" "$ARCHIVE.sha256")'`},
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
