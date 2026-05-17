# Per-phase CPU profiles

Baseline `pprof` top-20 summaries for the engine's five main phases,
captured against `testdata/static/wikipedia-latin-phrases.html` (~1.3 MB) — the
proven-stressful fixture from the latency-investigation work.

One baseline per benchmark:

- `BenchmarkPreprocess_Wikipedia.pprof.txt` — `PreprocessHTML`
- `BenchmarkSanitize_Wikipedia.pprof.txt`   — `SanitizeHTML`
- `BenchmarkCleanup_Wikipedia.pprof.txt`    — `CleanupMarkdown` (post-pass)
- `BenchmarkDedupe_Wikipedia.pprof.txt`     — `Dedupe`
- `BenchmarkSnapshot_Wikipedia.pprof.txt`   — `BuildSnapshot`

## Regenerate

All five at once:

    ./scripts/regen-bench-profiles.sh

Or per-bench:

    go test -run=^$ -bench=^BenchmarkSanitize_Wikipedia$ -benchtime=3x \
      -cpuprofile=/tmp/sanitize.prof ./internal/engine/
    go tool pprof -top -nodecount=20 /tmp/sanitize.prof \
      > tests/bench/profiles/BenchmarkSanitize_Wikipedia.pprof.txt

## How to use

When a PR shifts these baselines materially (different hot functions, a
big swing in `cum%`, or a new entry near the top), surface the diff in
review.

This is **NOT** a CI gate. Absolute numbers vary by machine and even by
run — `-benchtime=3x` is chosen to produce a usable profile, not tight
`ns/op` precision. The committed `.pprof.txt` files are a **review aid**
only.

If the Wikipedia fixture is ever regenerated, these baselines must be
regenerated too.

## Allocation budget gate

In addition to the human-readable pprof summaries above, this directory
also holds **`allocs_baseline.json`** — a machine-checked baseline of
`B/op` and `allocs/op` for the seven engine benchmarks:

- `BenchmarkExtract_Local`
- `BenchmarkFromHTML_WikipediaLatinPhrases`
- `BenchmarkPreprocess_Wikipedia`
- `BenchmarkSanitize_Wikipedia`
- `BenchmarkCleanup_Wikipedia`
- `BenchmarkDedupe_Wikipedia`
- `BenchmarkSnapshot_Wikipedia`

Unlike `ns/op` (which varies by machine), allocation counts are typically
deterministic for pure-CPU benchmarks, so the JSON is portable across
hosts.

### How the gate runs

`internal/engine/allocs_budget_test.go` is build-tag-gated by
`//go:build allocs && !race`. It re-runs each benchmark via
`testing.Benchmark(...)` and fails if `B/op` or `allocs/op` drifts by
more than the baseline's `tolerance_pct` (default **±15%**) in either
direction.

The test is **opt-in**, so it does **not** run under default
`go test ./...` or `./dev all`. Invoke it explicitly:

    go test -tags=allocs -run TestAllocationBudgets ./internal/engine/

The race detector inflates allocations dramatically, so the
`&& !race` half of the build tag prevents accidental `-race` runs from
producing misleading failures.

### Regenerate

After an intentional refactor that legitimately shifts allocations:

    ./scripts/regen-allocs-baseline.sh

Then commit the updated `allocs_baseline.json`. The script writes the
JSON in canonical bench order so diffs stay readable.

### Adding or removing a benchmark

The test verifies the baseline JSON and the
`registeredAllocBenches` map in `allocs_budget_test.go` agree in both
directions. After adding or removing a benchmark:

1. Update the `registeredAllocBenches` map.
2. Update the `BENCHES` array in `scripts/regen-allocs-baseline.sh`.
3. Re-run `scripts/regen-allocs-baseline.sh` and commit.
