package testserver

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Server is a simple HTTP server for serving test fixtures.
type Server struct {
	listener net.Listener
	server   *http.Server
	url      string
}

// Start creates and starts an HTTP server serving from testdata/static.
// Port 0 means the OS will choose an available port.
// Returns a Server handle for later cleanup.
func Start(port int) *Server {
	// Determine testdata path - look for testdata relative to project root
	var testdataPath string

	// Try current working directory first
	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}

	testdataPath = filepath.Join(cwd, "testdata")
	if _, err := os.Stat(testdataPath); err == nil {
		// Found it in cwd
	} else {
		// Try one level up (in case we're in a subdirectory)
		testdataPath = filepath.Join(cwd, "..", "testdata")
		if _, err := os.Stat(testdataPath); err == nil {
			// Found it one level up
		} else {
			// Try two levels up
			testdataPath = filepath.Join(cwd, "..", "..", "testdata")
			if _, err := os.Stat(testdataPath); err != nil {
				panic(fmt.Sprintf("testdata not found. Tried: %s, %s, %s",
					filepath.Join(cwd, "testdata"),
					filepath.Join(cwd, "..", "testdata"),
					filepath.Join(cwd, "..", "..", "testdata")))
			}
		}
	}

	// Create a listener on the specified port
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(fmt.Sprintf("failed to listen on port %d: %v", port, err))
	}

	// Get the actual port (useful when port 0 is provided)
	addr := listener.Addr().(*net.TCPAddr)
	actualPort := addr.Port
	baseURL := fmt.Sprintf("http://localhost:%d", actualPort)

	// Create HTTP server with file handler
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

	// Start server in a goroutine
	go func() {
		_ = httpServer.Serve(listener)
	}()

	return srv
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

// URL returns the base URL for the server (e.g., "http://localhost:8080").
func (s *Server) URL() string {
	return s.url
}
