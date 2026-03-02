package webui

import (
	"io/fs"
	"net/http"
)

// Handler returns an http.Handler that serves the embedded WebUI static files.
// It strips the "static" prefix so files are served from the root path.
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested file; if not found, serve index.html for SPA routing.
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// File not found — serve index.html for SPA client-side routing.
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
