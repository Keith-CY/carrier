package baseagent

type MemoryType string

const (
	MemoryTypePublic   MemoryType = "public"
	MemoryTypePerAgent MemoryType = "per_agent"
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
