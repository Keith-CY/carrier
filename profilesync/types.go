package profilesync

type AgentProfile struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	WorkspaceRoot   string   `json:"workspaceRoot,omitempty"`
	SkillReferences []string `json:"skillReferences,omitempty"`
}

type InstanceProfile struct {
	ID                string   `json:"id"`
	AgentID           string   `json:"agentId"`
	HostID            string   `json:"hostId"`
	ProviderProfileID string   `json:"providerProfileId,omitempty"`
	SyncMode          SyncMode `json:"syncMode"`
}

type SyncStatus struct {
	InstanceID       string `json:"instanceId"`
	DriftState       string `json:"driftState"`
	LastSyncAt       string `json:"lastSyncAt,omitempty"`
	LastSyncStatus   string `json:"lastSyncStatus,omitempty"`
	LastLocalCommit  string `json:"lastLocalCommit,omitempty"`
	LastRemoteCommit string `json:"lastRemoteCommit,omitempty"`
	LastCommonCommit string `json:"lastCommonCommit,omitempty"`
}

type ArtifactManifest struct {
	Runtime              string   `json:"runtime"`
	TrackedFiles         []string `json:"trackedFiles"`
	OptionalTrackedFiles []string `json:"optionalTrackedFiles,omitempty"`
	GeneratedFiles       []string `json:"generatedFiles,omitempty"`
	TransientFiles       []string `json:"transientFiles,omitempty"`
	SecretBearingFiles   []string `json:"secretBearingFiles,omitempty"`
}

type ReconcileReport struct {
	ConflictCount       int                    `json:"conflictCount"`
	AcceptedRemoteCount int                    `json:"acceptedRemoteCount"`
	InvalidRemoteCount  int                    `json:"invalidRemoteCount"`
	Conflicts           []string               `json:"conflicts,omitempty"`
	AcceptedRemotePaths []string               `json:"acceptedRemotePaths,omitempty"`
	ReconciledProfile   map[string]interface{} `json:"reconciledProfile"`
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
