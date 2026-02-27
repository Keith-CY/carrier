package profilesync

func DefaultPicoClawArtifactManifest() ArtifactManifest {
	return ArtifactManifest{
		Runtime: "picoclaw",
		TrackedFiles: []string{
			"~/.picoclaw/config.json",
		},
		GeneratedFiles: []string{
			"<workspace>/sessions",
			"<workspace>/memory",
			"<default-workspace>/state",
		},
		TransientFiles: []string{
			"*.lock",
			"tmp/*",
			"*.tmp",
		},
	}
}

func DefaultZeroClawArtifactManifest() ArtifactManifest {
	return ArtifactManifest{
		Runtime: "zeroclaw",
		TrackedFiles: []string{
			"~/.zeroclaw/config.toml",
		},
		OptionalTrackedFiles: []string{
			"~/.zeroclaw/agents.db",
		},
		SecretBearingFiles: []string{
			"~/.zeroclaw/.secret_key",
		},
		TransientFiles: []string{
			"*.bak",
			"*.tmp",
		},
	}
}
