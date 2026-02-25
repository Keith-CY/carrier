package gateway

import (
	"net/http"
	"sync"
)

var (
	webUIHandlerMu      sync.RWMutex
	webUIHandlerFactory = func() http.Handler { return http.NotFoundHandler() }
)

// SetWebUIHandlerFactory allows an embedding binary to inject a WebUI handler
// without creating a compile-time dependency from gateway -> webui.
func SetWebUIHandlerFactory(factory func() http.Handler) {
	webUIHandlerMu.Lock()
	defer webUIHandlerMu.Unlock()
	if factory == nil {
		webUIHandlerFactory = func() http.Handler { return http.NotFoundHandler() }
		return
	}
	webUIHandlerFactory = factory
}

func webUIHandler() http.Handler {
	webUIHandlerMu.RLock()
	factory := webUIHandlerFactory
	webUIHandlerMu.RUnlock()
	return factory()
}
