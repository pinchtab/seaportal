#!/usr/bin/env bash
# Regenerate per-phase pprof top-20 baselines under tests/bench/profiles/.
#
# Each benchmark is run with -benchtime=3x — enough to produce a meaningful
# CPU profile, not enough for tight ns/op precision. The committed *.pprof.txt
# files are a review aid, NOT a CI gate (perf varies by machine).
set -euo pipefail

cd "$(dirname "$0")/.."

OUT_DIR="tests/bench/profiles"
mkdir -p "$OUT_DIR"

BENCHES=(
  BenchmarkPreprocess_Wikipedia
  BenchmarkSanitize_Wikipedia
  BenchmarkCleanup_Wikipedia
  BenchmarkDedupe_Wikipedia
  BenchmarkSnapshot_Wikipedia
)

TMPDIR="${TMPDIR:-/tmp}"

for bench in "${BENCHES[@]}"; do
  prof="$TMPDIR/${bench}.prof"
  echo ">> $bench"
  go test -run=^$ -bench="^${bench}\$" -benchtime=3x \
    -cpuprofile="$prof" ./internal/engine/ > /dev/null
  go tool pprof -top -nodecount=20 "$prof" \
    > "$OUT_DIR/${bench}.pprof.txt"
done

echo "Wrote baselines to $OUT_DIR/"
