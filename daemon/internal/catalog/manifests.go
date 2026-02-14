package catalog

import "carrier/daemon/internal/manifest"

func OpenClawManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "./install.sh"},
			Upgrade: manifest.CommandSpec{Command: "./install.sh --upgrade"},
			Start:   manifest.CommandSpec{Command: "./openclaw --config ./config.yaml"},
			Stop:    manifest.CommandSpec{Command: "./openclaw --stop"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 8080}},
			Healthcheck: manifest.HealthcheckSpec{
				Type: "http",
				URL:  "http://localhost:8080/health",
			},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{{Name: "OPENAI_API_KEY", Secret: true}},
			Optional: []manifest.EnvVar{{Name: "LOG_LEVEL", Default: "info"}},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent, manifest.MemoryTypeShared, manifest.MemoryTypePublic},
			MountPath: "./memory",
		},
		Upgrade: manifest.UpgradeSpec{Channel: "stable", Strategy: "in_place_or_reinstall"},
		Diagnostics: manifest.Diagnostics{Include: []string{"runtime_logs", "process_state", "env_sanitized"}},
	}
}
