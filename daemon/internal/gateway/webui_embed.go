//go:build webui

package gateway

import (
	"net/http"

	"carrier/webui"
)

func webUIHandler() http.Handler {
	return webui.Handler()
}
