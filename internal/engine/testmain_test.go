package engine

import (
	"strings"
	"testing"
	"time"
)

// TestMain shrinks the exponential retry backoff base for the whole package so
// retry-path tests exercise the real logic without real-time sleeps (the
// production default is 1s; 5ms keeps relative growth assertions valid while
// cutting seconds off the suite). Retry-After-driven waits are unaffected —
// they sleep the header value, not this base.
func TestMain(m *testing.M) {
	retryBackoffBase = 5 * time.Millisecond
	m.Run()
}

// isHeavyFixture reports whether name is one of the large (hundreds of KB to
// >1 MB) real-world fixtures whose single FromHTML pass dominates wall time.
func isHeavyFixture(name string) bool {
	return strings.Contains(name, "wikipedia-latin") || strings.Contains(name, "github-awesome")
}

// skipHeavyFixture skips a test that drives a heavy real-world fixture through
// the full extraction pipeline. Under -race each pass runs ~20x slower (a 1.3 MB
// page takes ~50s vs ~2.4s), so these tests dominate the CI race lane while
// adding little race coverage the smaller fixtures don't already give. They run
// instead in the dedicated non-race "Heavy extraction fixtures" step (see
// .github/workflows/reusable-go.yml and scripts/test.sh) and under -short they
// are skipped from the fast inner-loop lane.
func skipHeavyFixture(t *testing.T) {
	t.Helper()
	switch {
	case testing.Short():
		t.Skip("skipping heavy fixture under -short")
	case isRaceEnabled:
		t.Skip("skipping heavy fixture under -race; covered by the non-race CI lane")
	}
}
