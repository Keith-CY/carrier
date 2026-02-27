package profilesync

func DefaultOpenClawArtifactManifest() ArtifactManifest {
	return ArtifactManifest{
		Runtime: "openclaw",
		TrackedFiles: []string{
			"openclaw.json",
			"AGENTS.md",
			"SOUL.md",
			"TOOLS.md",
			"IDENTITY.md",
			"USER.md",
			"HEARTBEAT.md",
			".openclaw/workspace-state.json",
		},
		OptionalTrackedFiles: []string{
			"BOOTSTRAP.md",
		},
		GeneratedFiles: []string{
			"models.json",
		},
		TransientFiles: []string{
			"*.lock",
			"tmp/*",
			"logs/*",
		},
		SecretBearingFiles: []string{
			"auth-profiles.json",
		},
	}
}
