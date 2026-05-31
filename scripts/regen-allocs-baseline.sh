#!/usr/bin/env bash
# Regenerate tests/bench/profiles/allocs_baseline.json from a fresh
# `go test -bench=.` run.
#
# Allocation counts (B/op, allocs/op) are deterministic across machines for
# pure-CPU benchmarks, so a single -benchtime=3x run is sufficient. The
# committed JSON is the baseline checked by `internal/engine/allocs_budget_test.go`
# (build tag `allocs`) with a ±15% drift tolerance.
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="tests/bench/profiles/allocs_baseline.json"
TMP_OUT="${TMPDIR:-/tmp}/allocs_bench.txt"

BENCHES=(
  BenchmarkExtract_Local
  BenchmarkFromHTML_WikipediaLatinPhrases
  BenchmarkPreprocess_Wikipedia
  BenchmarkSanitize_Wikipedia
  BenchmarkCleanup_Wikipedia
  BenchmarkDedupe_Wikipedia
  BenchmarkSnapshot_Wikipedia
)

# Pipe-separated regex of bench names anchored to whole-name match.
PATTERN="^($(IFS='|'; echo "${BENCHES[*]}"))\$"

echo ">> running benchmarks (-benchtime=3x)"
go test -run=^$ -bench="$PATTERN" -benchmem -benchtime=3x \
  ./internal/engine/ | tee "$TMP_OUT"

GO_VERSION="$(go version | awk '{print $3}')"
GOMAXPROCS="$(go env GOMAXPROCS 2>/dev/null || true)"
if [[ -z "$GOMAXPROCS" || "$GOMAXPROCS" == "0" ]]; then
  # Newer Go releases dropped GOMAXPROCS from `go env`. Parse it out of the
  # benchmark output instead (the -N suffix on each Benchmark line).
  GOMAXPROCS="$(awk '/^Benchmark/ { match($1, /-[0-9]+$/); if (RSTART) { print substr($1, RSTART+1, RLENGTH-1); exit } }' "$TMP_OUT")"
fi
if [[ -z "$GOMAXPROCS" ]]; then
  GOMAXPROCS=0
fi
CAPTURED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TOLERANCE_PCT=15
BENCHTIME="3x"

# Parse `go test -bench` output. Each bench line looks like:
#   BenchmarkX-12   3   1227486 ns/op   633482 B/op   4606 allocs/op
parsed="$(awk '
  /^Benchmark/ {
    # strip trailing -N from name
    name = $1
    sub(/-[0-9]+$/, "", name)
    # walk fields looking for "B/op" and "allocs/op"
    b_per_op = ""; allocs_per_op = ""
    for (i = 2; i <= NF; i++) {
      if ($i == "B/op")       b_per_op = $(i-1)
      if ($i == "allocs/op")  allocs_per_op = $(i-1)
    }
    if (b_per_op != "" && allocs_per_op != "") {
      printf "%s\t%s\t%s\n", name, b_per_op, allocs_per_op
    }
  }
' "$TMP_OUT")"

if [[ -z "$parsed" ]]; then
  echo "ERROR: no benchmark lines parsed from $TMP_OUT" >&2
  exit 1
fi

# Verify every expected bench appears.
for bench in "${BENCHES[@]}"; do
  if ! grep -q "^${bench}	" <<<"$parsed"; then
    echo "ERROR: expected benchmark $bench not found in output" >&2
    exit 1
  fi
done

mkdir -p "$(dirname "$OUT")"

{
  echo "{"
  echo "  \"version\": 1,"
  echo "  \"captured_at\": \"${CAPTURED_AT}\","
  echo "  \"go_version\": \"${GO_VERSION}\","
  echo "  \"gomaxprocs\": ${GOMAXPROCS},"
  echo "  \"benchtime\": \"${BENCHTIME}\","
  echo "  \"tolerance_pct\": ${TOLERANCE_PCT},"
  echo "  \"benchmarks\": {"
  first=1
  # Emit entries in the canonical BENCHES order for stable diffs.
  for bench in "${BENCHES[@]}"; do
    line="$(grep "^${bench}	" <<<"$parsed")"
    b_per_op="$(cut -f2 <<<"$line")"
    allocs_per_op="$(cut -f3 <<<"$line")"
    if [[ $first -eq 1 ]]; then
      first=0
    else
      echo ","
    fi
    printf '    "%s": {"b_per_op": %s, "allocs_per_op": %s}' \
      "$bench" "$b_per_op" "$allocs_per_op"
  done
  echo ""
  echo "  }"
  echo "}"
} > "$OUT"

echo "Wrote $OUT"
