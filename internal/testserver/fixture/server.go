// Package fixture provides a declarative, in-process HTTP test server for
// per-test scenarios that exercise HTTP-level behaviour (status codes,
// redirects, slow responses, charset declarations, header echoes, etc.).
//
// Lifecycle — "build then go":
//
//	srv := fixture.New().
//	    Route("GET", "/status/406", fixture.Status(406)).
//	    Route("GET", "/redirect",   fixture.Redirect("/final", 301)).
//	    Route("GET", "/final",      fixture.Body([]byte("ok"), "text/plain"))
//	defer srv.Close()
//	resp, err := http.Get(srv.URL() + "/redirect")
//
// Call Route as many times as needed before the first call to URL(). Each
// Route mutates the underlying mux directly; once URL() is read, the server
// is considered live and additional Route calls — while not forbidden by
// the runtime — are racy and should be avoided.
//
// The server binds to 127.0.0.1 via net/http/httptest and chooses a free
// port automatically. It only depends on the standard library and serves
// plain HTTP (no TLS). For TLS scenarios add a NewTLS constructor later.
package fixture

import (
	"net/http"
	"net/http/httptest"
)

// Server wraps an *httptest.Server and exposes a fluent route DSL.
type Server struct {
	mux  *http.ServeMux
	http *httptest.Server
}

// New constructs a fixture Server with an empty mux and starts the
// underlying httptest.Server immediately so URL() is valid right away.
func New() *Server {
	mux := http.NewServeMux()
	return &Server{
		mux:  mux,
		http: httptest.NewServer(mux),
	}
}

// Route registers a handler for the given method and path. The method is
// matched exactly; mismatched methods receive a 405 response.
//
// Routing uses Go 1.22+ method-prefixed ServeMux patterns
// (e.g. "GET /status/406"), which gives free 405 handling for the same
// path bound to a different method.
func (s *Server) Route(method, path string, h http.HandlerFunc) *Server {
	pattern := method + " " + path
	s.mux.HandleFunc(pattern, h)
	return s
}

// URL returns the base URL of the running server, e.g. "http://127.0.0.1:54321".
func (s *Server) URL() string {
	return s.http.URL
}

// Close stops the underlying httptest.Server and releases its port.
func (s *Server) Close() {
	s.http.Close()
}
