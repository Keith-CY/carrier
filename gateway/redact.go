// Package gateway provides the carrier gateway HTTP server that bridges
// messaging platform webhooks and commands to the daemon API.
package gateway

import (
	"carrier/shared/redact"
)

// RedactErrorMessage redacts sensitive details from user-facing errors.
// It masks key/value tokens (API_KEY/SECRET/TOKEN/PASSWORD/CREDENTIAL) and
// URL-embedded credentials, then delegates to the shared redact package so
// daemon/gateway apply the same redaction boundary.
func RedactErrorMessage(message string) string {
	return redact.RedactText(message)
}
