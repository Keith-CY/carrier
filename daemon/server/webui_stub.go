//go:build !webui

package server

import "net/http"

func webUIHandler() http.Handler {
	return http.NotFoundHandler()
}
