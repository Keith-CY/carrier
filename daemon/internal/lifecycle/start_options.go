package lifecycle

// StartOptions configures runtime start behavior.
type StartOptions struct {
	// Isolation requests sandboxed runtime execution for this start operation.
	// When true and isolation backend is unavailable, start must fail fast.
	Isolation bool `json:"isolation,omitempty"`
}
