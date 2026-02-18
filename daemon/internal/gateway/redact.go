// Package gateway provides the carrier gateway HTTP server that bridges
// messaging platform webhooks and commands to the daemon API.
package gateway

import (
	"carrier/daemon/internal/redact"
)

// RedactErrorMessage redacts sensitive information from error messages.
// Delegates to the shared redact package for consistent behavior across
// daemon and gateway.
func RedactErrorMessage(message string) string {
	return redact.RedactText(message)
}
