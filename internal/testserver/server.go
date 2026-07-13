package testserver

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Server struct {
	listener net.Listener
	server   *http.Server
	url      string
}

func Start(port int) *Server {
	var testdataPath string

	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}

	testdataPath = filepath.Join(cwd, "testdata")
	if _, err := os.Stat(testdataPath); err == nil {
	} else {
		testdataPath = filepath.Join(cwd, "..", "testdata")
		if _, err := os.Stat(testdataPath); err == nil {
		} else {
			testdataPath = filepath.Join(cwd, "..", "..", "testdata")
			if _, err := os.Stat(testdataPath); err != nil {
				panic(fmt.Sprintf("testdata not found. Tried: %s, %s, %s",
					filepath.Join(cwd, "testdata"),
					filepath.Join(cwd, "..", "testdata"),
					filepath.Join(cwd, "..", "..", "testdata")))
			}
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(fmt.Sprintf("failed to listen on port %d: %v", port, err))
	}

	addr := listener.Addr().(*net.TCPAddr)
	actualPort := addr.Port
	baseURL := fmt.Sprintf("http://localhost:%d", actualPort)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(testdataPath)))

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	srv := &Server{
		listener: listener,
		server:   httpServer,
		url:      baseURL,
	}

	go func() {
		_ = httpServer.Serve(listener)
	}()

	return srv
}

func (s *Server) Stop() {
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func (s *Server) URL() string {
	return s.url
}
