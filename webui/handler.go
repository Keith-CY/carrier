package webui

import (
	"io/fs"
	"net/http"
)

// Handler returns an http.Handler that serves the embedded WebUI static files.
// It strips the "static" prefix so files are served from the root path.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("webui: failed to create sub filesystem: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file; if not found, serve index.html for SPA routing
		fileServer.ServeHTTP(w, r)
	})
}
