//go:build !webui

package gateway

import "net/http"

func webUIHandler() http.Handler {
	return http.NotFoundHandler()
}
