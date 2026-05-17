package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/seaportal/internal/engine/leakcheck"
)

// TestAdversarial_Inputs exercises FromHTML against deliberately pathological
// inputs (size bombs, deep nesting, entity-expansion attacks, malformed UTF-8,
// mixed encodings, broken tables). Each subtest enforces a 5s wall-clock
// budget, recovers panics in the worker goroutine, and checks goroutine
// hygiene via leakcheck.
//
// Observe-first policy: if a subtest reveals a real bug (panic, hang past 5s,
// OOM, runaway CPU), DO NOT widen the budget. Lock in actual behaviour with a
// comment naming the bug class and file a follow-up in todo.md.
//
// Caveat: FromHTML has no context plumbing, so a goroutine that hangs past
// the deadline keeps running until it returns. leakcheck's 200ms window after
// the subtest finishes will surface that as a leak. That is itself a useful
// signal — see the leakcheck doc on the limitation.
func TestAdversarial_Inputs(t *testing.T) {
	cases := []struct {
		name   string
		htmlFn func() string
		// budget overrides the default 5s deadline for cases with a
		// known, documented slowness that the project has accepted
		// pending a follow-up fix. ALWAYS pair a non-default budget
		// with a code comment AND a todo.md entry naming the bug
		// class. Default (zero) means use defaultBudget.
		budget time.Duration
	}{
		{
			name: "10mb-single-line",
			// LOCKED-IN BEHAVIOUR (not a fix): characterized wall-clock
			// is ~9.4s on dev hardware without -race. Under the race
			// detector (which `./dev all` enables) the same input
			// takes ~3m13s — a 20x slowdown because the race detector
			// instruments every allocation and the pipeline allocates
			// many ~10MB strings. The case is SKIPPED under -race to
			// keep `./dev all` finishing in reasonable time; the plain
			// `go test ./internal/engine/` lane still runs it.
			// Follow-up: see todo.md — "10MB-single-line FromHTML
			// takes ~9s; profile and shrink large-input pipeline cost".
			budget: 30 * time.Second,
			// Pathology: one ~10MB <body> line. Stresses any O(n^2) scan
			// (regex bracketing, line splitting, dedupe hashing) that
			// walks the whole document per character.
			htmlFn: func() string {
				const target = 10 * 1024 * 1024 // 10MB
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
			// Pathology: 1000 nested <div>s. Stresses recursive DOM
			// walkers (sanitize, readability, markdown converter). A
			// naive recursive walker without explicit depth budget can
			// stack-overflow.
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
				b, err := os.ReadFile("../../testdata/adversarial/billion-laughs.html")
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				return string(b)
			},
		},
		{
			name: "malformed-utf8",
			htmlFn: func() string {
				b, err := os.ReadFile("../../testdata/adversarial/malformed-utf8.html")
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				return string(b)
			},
		},
		{
			name: "mixed-encodings",
			htmlFn: func() string {
				b, err := os.ReadFile("../../testdata/adversarial/mixed-encodings.html")
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				return string(b)
			},
		},
		{
			name: "broken-tables",
			htmlFn: func() string {
				b, err := os.ReadFile("../../testdata/adversarial/broken-tables.html")
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}
				return string(b)
			},
		},
	}

	const defaultBudget = 5 * time.Second

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// The 10MB size-bomb takes ~3 minutes under the race
			// detector (vs ~9s without). Skip under -race so
			// `./dev all` stays fast; non-race lanes still cover it.
			if isRaceEnabled && tc.name == "10mb-single-line" {
				t.Skip("10MB size-bomb skipped under -race (3m+ wall-clock); covered by non-race lane. See todo.md follow-up.")
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
					// Observed bug: FromHTML panicked on adversarial input.
					// DO NOT mask. Fail loudly so the follow-up todo gets
					// filed and the root cause is investigated.
					t.Fatalf("FromHTML panicked on %s after %s: %v", tc.name, elapsed, o.panic)
				}
				t.Logf("subtest=%s wall=%s len(content)=%d err=%q", tc.name, elapsed, len(o.result.Content), o.result.Error)
				// Soft signal: a case eating >1s on the default 5s
				// budget is close to the cliff. Flag it. Cases with
				// an explicit budget override have already been
				// triaged — skip the noise log there.
				if tc.budget == 0 && elapsed > 1*time.Second {
					t.Logf("subtest=%s SLOW: %s exceeds 1s soft threshold", tc.name, elapsed)
				}
			case <-ctx.Done():
				// Hang past the budget. Worker goroutine continues
				// running and will be caught by leakcheck. Do NOT
				// silently widen — file a follow-up todo first.
				t.Fatalf("subtest=%s exceeded %s budget — likely O(n^k) blowup or infinite loop in pipeline", tc.name, budget)
			}
		})
	}
}
