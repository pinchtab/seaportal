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

# When called with no target, run all first-party packages (excluding
# competitors/ which is third-party vendor code we don't own).
if [ -n "${1:-}" ]; then
  TARGET="$1"
else
  TARGET=$(go list ./... 2>/dev/null | grep -v '/competitors/' | tr '\n' ' ')
  [ -z "$TARGET" ] && TARGET="./..."
fi

echo ""
echo -e "  ${ACCENT}${BOLD}🌊 SeaPortal Tests${NC}"
echo -e "  ${MUTED}Target: $TARGET${NC}"

# ── Tests ────────────────────────────────────────────────────────────

section "Running Tests"

resolve_gotestsum() {
  if command -v gotestsum >/dev/null 2>&1; then
    command -v gotestsum
    return 0
  fi
  local gobin gopath
  gobin="$(go env GOBIN 2>/dev/null)"
  if [ -n "$gobin" ] && [ -x "$gobin/gotestsum" ]; then
    echo "$gobin/gotestsum"
    return 0
  fi
  gopath="$(go env GOPATH 2>/dev/null)"
  if [ -n "$gopath" ] && [ -x "$gopath/bin/gotestsum" ]; then
    echo "$gopath/bin/gotestsum"
    return 0
  fi
  return 1
}

if GOTESTSUM_BIN="$(resolve_gotestsum)"; then
  if ! "$GOTESTSUM_BIN" --format=pkgname --hide-summary=output -- -count=1 -race -coverprofile=coverage.out $TARGET; then
    fail "Tests failed"
    exit 1
  fi
else
  echo -e "    ${MUTED}gotestsum not found — falling back to go test${NC}"
  echo -e "    ${MUTED}Install: go install gotest.tools/gotestsum@latest${NC}"
  echo ""
  if ! go test -v -count=1 -race -coverprofile=coverage.out $TARGET; then
    fail "Tests failed"
    exit 1
  fi
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
