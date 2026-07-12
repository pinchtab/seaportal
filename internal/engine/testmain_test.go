package engine

import (
	"strings"
	"testing"
	"time"
)

// testRetryBackoffBase is the shrunk exponential backoff unit retry-path
// tests set on Options.RetryBackoffBase (T16 — the former package-global
// retryBackoffBase/TestMain shrink): real retry logic, no real-time sleeps.
// 5ms keeps relative growth assertions valid. Retry-After-driven waits are
// unaffected — they sleep the header value, not this base.
const testRetryBackoffBase = 5 * time.Millisecond

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
