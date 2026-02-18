//go:build webui

package server

import (
	"net/http"

	"carrier/webui"
)

func webUIHandler() http.Handler {
	return webui.Handler()
}
