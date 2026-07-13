package fixture

import (
	"net/http"
	"net/http/httptest"
)

type Server struct {
	mux  *http.ServeMux
	http *httptest.Server
}

func New() *Server {
	mux := http.NewServeMux()
	return &Server{
		mux:  mux,
		http: httptest.NewServer(mux),
	}
}

func (s *Server) Route(method, path string, h http.HandlerFunc) *Server {
	pattern := method + " " + path
	s.mux.HandleFunc(pattern, h)
	return s
}

func (s *Server) URL() string {
	return s.http.URL
}

func (s *Server) Close() {
	s.http.Close()
}
