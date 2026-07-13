package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

func TestAdversarial_Inputs(t *testing.T) {
	cases := []struct {
		name   string
		htmlFn func() string
		budget time.Duration
	}{
		{
			name:   "10mb-single-line",
			budget: 30 * time.Second,
			htmlFn: func() string {
				const target = 10 * 1024 * 1024
				chunk := "the quick brown fox jumps over the lazy dog. "
				reps := target / len(chunk)
				var sb strings.Builder
				sb.Grow(target + 256)
				sb.WriteString("<!DOCTYPE html><html><head><title>10MB single line</title></head><body><p>")
				sb.WriteString(strings.Repeat(chunk, reps))
				sb.WriteString("</p></body></html>")
				return sb.String()
			},
		},
		{
			name: "deep-nesting",
			htmlFn: func() string {
				const depth = 1000
				var sb strings.Builder
				sb.Grow(depth * 12)
				sb.WriteString("<!DOCTYPE html><html><head><title>deep</title></head><body>")
				for i := 0; i < depth; i++ {
					sb.WriteString("<div>")
				}
				sb.WriteString("<p>innermost paragraph with some meaningful prose content so readability has something to chew on.</p>")
				for i := 0; i < depth; i++ {
					sb.WriteString("</div>")
				}
				sb.WriteString("</body></html>")
				return sb.String()
			},
		},
		{
			name: "billion-laughs",
			htmlFn: func() string {
				return loadFixture(t, "adversarial/billion-laughs.html")
			},
		},
		{
			name: "malformed-utf8",
			htmlFn: func() string {
				return loadFixture(t, "adversarial/malformed-utf8.html")
			},
		},
		{
			name: "mixed-encodings",
			htmlFn: func() string {
				return loadFixture(t, "adversarial/mixed-encodings.html")
			},
		},
		{
			name: "broken-tables",
			htmlFn: func() string {
				return loadFixture(t, "adversarial/broken-tables.html")
			},
		},
	}

	const defaultBudget = 5 * time.Second

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "10mb-single-line" && (isRaceEnabled || testing.Short()) {
				t.Skip("10MB size-bomb skipped under -race (3m+ wall-clock) and -short (fast lane); covered by the full non-race lane.")
			}
			leakcheck.CheckLeak(t)

			html := tc.htmlFn()
			url := "https://example.invalid/" + tc.name

			budget := tc.budget
			if budget == 0 {
				budget = defaultBudget
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()

			type outcome struct {
				result Result
				panic  any
			}
			done := make(chan outcome, 1)

			startWall := time.Now()
			go func() {
				var o outcome
				defer func() {
					if r := recover(); r != nil {
						o.panic = r
					}
					done <- o
				}()
				o.result = FromHTML(html, url)
			}()

			select {
			case o := <-done:
				elapsed := time.Since(startWall)
				if o.panic != nil {
					t.Fatalf("FromHTML panicked on %s after %s: %v", tc.name, elapsed, o.panic)
				}
				t.Logf("subtest=%s wall=%s len(content)=%d err=%q", tc.name, elapsed, len(o.result.Content), o.result.Error)
				if tc.budget == 0 && elapsed > 1*time.Second {
					t.Logf("subtest=%s SLOW: %s exceeds 1s soft threshold", tc.name, elapsed)
				}
			case <-ctx.Done():
				t.Fatalf("subtest=%s exceeded %s budget — likely O(n^k) blowup or infinite loop in pipeline", tc.name, budget)
			}
		})
	}
}
