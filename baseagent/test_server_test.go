package baseagent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newLocalhostServer forces IPv4 loopback to avoid IPv6 binding issues in restricted environments.
func newLocalhostServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}

	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
