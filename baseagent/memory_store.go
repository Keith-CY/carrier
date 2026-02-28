package baseagent

type MemoryType string

const (
	MemoryTypePublic   MemoryType = "public"
	MemoryTypePerAgent MemoryType = "per_agent"
	MemoryTypeShared   MemoryType = "shared"
)

type MemoryState string

const (
	MemoryStateArchived MemoryState = "archived"
)

type MemoryEntry struct {
	ID    string
	State MemoryState
}

type ExportOptions struct {
	Actor     string
	RequestID string
}

type MemorySearchHit struct {
	ID         string
	Scope      string
	Score      float64
	Snippet    string
	Provenance string
}

type MemoryRecord struct {
	ID             string
	Scope          string
	Type           string
	ContentRaw     string
	ContentSummary string
	Provenance     string
}

type MemoryAudit struct {
	Action    string
	Target    string
	Result    string
	Message   string
	Timestamp string
}

// MemoryStore abstracts daemon memory persistence for the base agent runtime.
type MemoryStore interface {
	Get(id string) error
	Create(id, name, version string, memType MemoryType, owner string) error
	List() []MemoryEntry
	SetAttachmentsFromLinks(agentID string, memoryIDs []string) error
	PrepareAgentMemory(agentID string) error
	ExportMemory(memoryID string, opts ExportOptions) (string, error)
	Archive(memoryID string) error
}

// ExtendedMemoryStore provides FusionMem methods without breaking legacy callers.
type ExtendedMemoryStore interface {
	MemoryStore
	Search(subject, query string, maxResults int, minScore float64) ([]MemorySearchHit, error)
	GetRecord(subject, id string) (MemoryRecord, error)
	Observe(subject, toolName, outputSnippet, scope string, autoCurate bool) (string, error)
	Grant(subject, scope, grantedBy, reason string) (string, error)
	Revoke(grantID, revokedBy string) error
	ListAudits() []MemoryAudit
}
