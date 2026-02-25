//go:build webui

package main

import (
	"net/http"

	gatewayruntime "carrier/gateway"
	"carrier/webui"
)

func init() {
	gatewayruntime.SetWebUIHandlerFactory(func() http.Handler {
		return webui.Handler()
	})
}

