// Package gateway exposes the daemon gateway runtime as a callable Run function.
// This wrapper keeps cmd/carrier decoupled from internal package paths.
package gateway

import (
	internalgateway "carrier/daemon/internal/gateway"
)

// Run starts the gateway HTTP server using environment-based configuration.
func Run() error {
	return internalgateway.StartGateway(nil)
}
