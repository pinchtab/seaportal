package fixture

import (
	"net/http"
	"time"
)

// Status returns a handler that writes the given HTTP status code with an
// empty body. Useful for /status/406, /status/429, etc.
func Status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

// Redirect returns a handler that writes a Location header and the given
// redirect status code (e.g. 301, 302, 307, 308). No body is written so
// downstream cache/dedupe logic doesn't see a redirect blob.
func Redirect(location string, code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", location)
		w.WriteHeader(code)
	}
}

// Delay wraps inner with a time.Sleep before delegating. Composable with
// any other handler — Delay(2*time.Second, Status(200)) yields a slow 200.
func Delay(d time.Duration, inner http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(d)
		inner.ServeHTTP(w, r)
	}
}

// Charset returns a handler that serves body verbatim with
// Content-Type: text/html; charset=<label>. The label is written exactly
// as given (e.g. "iso-8859-1", "UTF-8", "windows-1252") so engine charset
// detection has a real declared encoding to react to.
func Charset(label string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset="+label)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// Body returns a handler that writes body with the given Content-Type and
// a 200 status. Passing an empty contentType skips the header (letting
// Go's default content sniffing apply).
func Body(body []byte, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// Headers wraps inner with extra response headers. Headers from h are
// added (not replaced) before inner runs so inner can still override
// individual values via w.Header().Set.
func Headers(h http.Header, inner http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for k, vs := range h {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		inner.ServeHTTP(w, r)
	}
}
