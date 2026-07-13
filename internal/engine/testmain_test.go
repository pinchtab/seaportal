package engine

import (
	"strings"
	"testing"
	"time"
)

const testRetryBackoffBase = 5 * time.Millisecond

func isHeavyFixture(name string) bool {
	return strings.Contains(name, "wikipedia-latin") || strings.Contains(name, "github-awesome")
}

func skipHeavyFixture(t *testing.T) {
	t.Helper()
	switch {
	case testing.Short():
		t.Skip("skipping heavy fixture under -short")
	case isRaceEnabled:
		t.Skip("skipping heavy fixture under -race; covered by the non-race CI lane")
	}
}
