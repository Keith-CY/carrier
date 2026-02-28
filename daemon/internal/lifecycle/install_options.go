package lifecycle

// InstallOptions configures runtime install behavior.
type InstallOptions struct {
	// Isolation requests install execution through the selected isolation
	// backend context for this operation.
	Isolation bool `json:"isolation,omitempty"`
}
