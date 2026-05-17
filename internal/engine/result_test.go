package engine

import (
	"encoding/json"
	"testing"
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
