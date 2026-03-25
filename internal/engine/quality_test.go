package engine

import (
	"testing"
)

// TestComputeQuality_Delegation verifies the extract package delegates to quality package
func TestComputeQuality_Delegation(t *testing.T) {
	md := "# Title\n\nSome content paragraph with enough text to be meaningful.\n\n## Section\n\n- Item 1\n- Item 2\n\nAnother paragraph here."
	m := ComputeQuality(md)
	if m.Score <= 0 {
		t.Errorf("Expected positive score, got %f", m.Score)
	}
	if m.ContentLength != len(md) {
		t.Errorf("ContentLength = %d, want %d", m.ContentLength, len(md))
	}
}
