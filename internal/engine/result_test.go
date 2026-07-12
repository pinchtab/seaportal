package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResult_JSONHasTopLevelPageClass(t *testing.T) {
	r := &Result{
		URL:        "https://example.com",
		Confidence: 90,
		Length:     2500,
		Profile: PageProfile{
			Class:       PageStatic,
			Outcome:     OutcomeExtract,
			Trustworthy: true,
			Confidence:  90,
		},
	}
	ensureProfile(r)

	if r.PageClass != PageStatic {
		t.Fatalf("PageClass on struct = %q, want %q", r.PageClass, PageStatic)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	top, ok := m["pageClass"].(string)
	if !ok {
		t.Fatalf("pageClass not present at top level or not a string: %#v", m["pageClass"])
	}
	if top != "static" {
		t.Errorf("top-level pageClass = %q, want %q", top, "static")
	}

	profile, ok := m["profile"].(map[string]any)
	if !ok {
		t.Fatalf("profile not present as object")
	}
	nested, _ := profile["class"].(string)
	if nested != top {
		t.Errorf("profile.class = %q, top pageClass = %q; should match", nested, top)
	}
}

func TestResult_PageClassPopulatedOnErrorPath(t *testing.T) {
	r := &Result{Error: "boom"}
	ensureProfile(r)

	if r.PageClass == "" {
		t.Fatalf("PageClass empty on error-path Result; want non-empty")
	}
	if r.PageClass != r.Profile.Class {
		t.Errorf("PageClass=%q, Profile.Class=%q; should be in lockstep", r.PageClass, r.Profile.Class)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	top, _ := m["pageClass"].(string)
	if top == "" {
		t.Errorf("top-level pageClass empty in JSON")
	}
}

// T17: Result.Err() must preserve sentinel identity through real fetch paths
// — the JSON Error string alone forces consumers into string matching.
func TestResultErr_PreservesSentinels(t *testing.T) {
	t.Run("ssrf-blocked private IP", func(t *testing.T) {
		// Literal loopback IP: ValidateURL blocks it without any DNS lookup.
		res := FromURLWithOptions("http://127.0.0.1:1/", Options{Security: DefaultSecurityPolicy()})
		if res.Error == "" || res.SecurityBlock == "" {
			t.Fatalf("expected a security block, got error=%q securityBlock=%q", res.Error, res.SecurityBlock)
		}
		if !errors.Is(res.Err(), ErrPrivateIPBlocked) {
			t.Errorf("errors.Is(res.Err(), ErrPrivateIPBlocked) = false; Err() = %v", res.Err())
		}
		if res.Err().Error() != res.Error {
			t.Errorf("Err().Error() = %q must equal the serialized Error %q", res.Err().Error(), res.Error)
		}
	})

	t.Run("blocked by robots.txt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				_, _ = w.Write([]byte("User-agent: *\nDisallow: /private/\n"))
				return
			}
			_, _ = w.Write([]byte("<html><body><p>secret</p></body></html>"))
		}))
		defer srv.Close()

		res := FromURLWithOptions(srv.URL+"/private/page", Options{RespectRobots: true})
		if !res.BlockedByRobots {
			t.Fatalf("expected BlockedByRobots, got error=%q", res.Error)
		}
		if !errors.Is(res.Err(), ErrBlockedByRobots) {
			t.Errorf("errors.Is(res.Err(), ErrBlockedByRobots) = false; Err() = %v", res.Err())
		}
		if res.Error != ErrBlockedByRobots.Error() {
			t.Errorf("Error string changed: %q, want %q", res.Error, ErrBlockedByRobots.Error())
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := FromURLWithOptions("http://192.0.2.1/", Options{Context: ctx, RateLimit: time.Second})
		if res.Error == "" {
			t.Fatal("expected an error from the pre-cancelled context")
		}
		if !errors.Is(res.Err(), context.Canceled) {
			t.Errorf("errors.Is(res.Err(), context.Canceled) = false; Err() = %v", res.Err())
		}
	})

	t.Run("success yields nil", func(t *testing.T) {
		var r Result
		if r.Err() != nil {
			t.Errorf("zero Result.Err() = %v, want nil", r.Err())
		}
	})

	t.Run("bare string Error still surfaces", func(t *testing.T) {
		r := Result{Error: "hand-assigned"}
		if r.Err() == nil || r.Err().Error() != "hand-assigned" {
			t.Errorf("Err() = %v, want opaque error matching the string", r.Err())
		}
	})

	t.Run("unexported err never serializes", func(t *testing.T) {
		var r Result
		r.setError(errors.New("wire-invisible"))
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if m["error"] != "wire-invisible" {
			t.Errorf("error key = %v, want the flattened string", m["error"])
		}
		if _, ok := m["err"]; ok {
			t.Error("unexported err leaked into JSON")
		}
	})
}
