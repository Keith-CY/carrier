package memory

import "time"

// Scope is an access namespace (for example: agent:<id>, shared:<name>, public).
type Scope string

const (
	ScopePublic Scope = "public"
)

// RecordType classifies a stable memory record.
type RecordType string

const (
	RecordTypePreference RecordType = "preference"
	RecordTypeFact       RecordType = "fact"
	RecordTypeDecision   RecordType = "decision"
	RecordTypeTaskState  RecordType = "task_state"
	RecordTypeNote       RecordType = "note"
)

// MemoryRecord is a curated memory unit that can be injected into context.
type MemoryRecord struct {
	ID             string     `json:"id"`
	Scope          Scope      `json:"scope"`
	Type           RecordType `json:"type"`
	ContentRaw     string     `json:"content_raw,omitempty"`
	ContentSummary string     `json:"content_summary"`
	Tags           []string   `json:"tags,omitempty"`
	Provenance     string     `json:"provenance,omitempty"`
	Confidence     float64    `json:"confidence,omitempty"`
	Importance     int        `json:"importance,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
}

// ObservationEvent is an append-only ledger event from runtime actions/tools.
type ObservationEvent struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	AgentID       string    `json:"agent_id,omitempty"`
	AppID         string    `json:"app_id,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	Scope         Scope     `json:"scope"`
	ToolName      string    `json:"tool_name,omitempty"`
	InputsDigest  string    `json:"inputs_digest,omitempty"`
	OutputSnippet string    `json:"output_snippet,omitempty"`
	Status        string    `json:"status,omitempty"`
	Artifacts     []string  `json:"artifacts,omitempty"`
	Labels        []string  `json:"labels,omitempty"`
}

// Grant represents explicit authorization from a subject to a scope.
type Grant struct {
	ID        string     `json:"id"`
	Subject   string     `json:"subject"`
	Scope     Scope      `json:"scope"`
	GrantedBy string     `json:"granted_by,omitempty"`
	GrantedAt time.Time  `json:"granted_at"`
	Reason    string     `json:"reason,omitempty"`
	RevokedBy string     `json:"revoked_by,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// SearchOptions defines query and authorization context for memory search.
type SearchOptions struct {
	Subject    string
	Query      string
	MaxResults int
	MinScore   float64

	// CandidateMultiplier controls how many candidates are pulled for reranking.
	// Larger values increase recall at the cost of search-time work.
	CandidateMultiplier int
	// AdaptiveRecall enables secondary in-memory fallback when SQLite returns sparse results.
	AdaptiveRecall *bool
	// Rerank enables token-overlap + lexical score fusion for final ranking.
	Rerank *bool
	// LexicalWeight controls the fusion weight for SQLite/full-text score.
	LexicalWeight *float64
	// OverlapWeight controls the fusion weight for token-overlap score.
	// (Named "overlap" rather than "semantic" since this is token matching, not embedding-based.)
	OverlapWeight *float64
}

// SearchHit is the compact result from progressive search.
type SearchHit struct {
	ID         string  `json:"id"`
	Scope      Scope   `json:"scope"`
	Score      float64 `json:"score"`
	Snippet    string  `json:"snippet"`
	Provenance string  `json:"provenance,omitempty"`
}

// ObserveInput captures tool execution traces in the ledger.
type ObserveInput struct {
	Subject       string
	AgentID       string
	AppID         string
	SessionID     string
	Scope         Scope
	ToolName      string
	InputsDigest  string
	OutputSnippet string
	Status        string
	Artifacts     []string
	Labels        []string
	AutoCurate    bool
}

// UpsertRecordInput writes or updates a curated memory record.
type UpsertRecordInput struct {
	Subject        string
	ID             string
	Scope          Scope
	Type           RecordType
	ContentRaw     string
	ContentSummary string
	Tags           []string
	Provenance     string
	Confidence     float64
	Importance     int
}

// InstanceImportOptions controls legacy and truth-file imports.
type InstanceImportOptions struct {
	Actor       string
	RequestID   string
	TargetScope Scope
}

// InstanceExportOptions controls instance export packaging.
type InstanceExportOptions struct {
	Actor     string
	RequestID string
	Format    string // truth-only | truth+index
}
