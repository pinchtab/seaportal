#!/bin/bash
set -e

# test.sh — Run Go tests with coverage
# Usage: test.sh [package]

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

ok()   { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail() { echo -e "  ${ERROR}✗${NC} $1"; }

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}

TARGET="${1:-./...}"

echo ""
echo -e "  ${ACCENT}${BOLD}🌊 SeaPortal Tests${NC}"
echo -e "  ${MUTED}Target: $TARGET${NC}"

# ── Tests ────────────────────────────────────────────────────────────

section "Running Tests"

if ! go test -v -count=1 -race -coverprofile=coverage.out "$TARGET"; then
  fail "Tests failed"
  exit 1
fi
ok "All tests passed"

# ── Coverage ─────────────────────────────────────────────────────────

section "Coverage"

go tool cover -func=coverage.out | tail -10

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
TOTAL=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}')
echo ""
echo -e "  ${SUCCESS}${BOLD}Total coverage: $TOTAL${NC}"
echo ""

rm -f coverage.out
