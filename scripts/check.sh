#!/bin/bash
set -e

# check.sh — Local pre-push checks matching GitHub Actions CI
# Runs: format → vet → build → lint

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

ok()   { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail() { echo -e "  ${ERROR}✗${NC} $1"; [ -n "${2:-}" ] && echo -e "    ${MUTED}$2${NC}"; }
hint() { echo -e "    ${MUTED}$1${NC}"; }

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}

trap 'rm -f seaportal coverage.out 2>/dev/null' EXIT

echo ""
echo -e "  ${ACCENT}${BOLD}🌊 SeaPortal Check${NC}"
echo -e "  ${MUTED}Running pre-push checks (matches GitHub Actions CI)...${NC}"

# ── Format ───────────────────────────────────────────────────────────

section "Format"

# Format/vet/lint targets exclude competitors/ (third-party vendor code) and
# vendor/. Use find with prune so the exclusion is consistent across tools.
GO_FILES=$(find . \( -path './competitors' -o -path './vendor' -o -path './.git' -o -path './node_modules' \) -prune -o -name '*.go' -type f -print)
unformatted=$(echo "$GO_FILES" | xargs gofmt -l 2>/dev/null)
if [ -n "$unformatted" ]; then
  fail "gofmt" "Files not formatted:"
  echo "$unformatted" | while read f; do hint "  $f"; done
  echo ""
  printf "  Fix formatting now? (Y/n) "
  read -r answer
  if [ "$answer" != "n" ] && [ "$answer" != "N" ]; then
    echo "$GO_FILES" | xargs gofmt -w
    ok "gofmt (fixed)"
  else
    hint "Run: gofmt -w <files>  (excluding competitors/)"
    exit 1
  fi
else
  ok "gofmt"
fi

# ── Vet ──────────────────────────────────────────────────────────────

section "Vet"

# Limit to first-party packages; competitors/ is third-party vendor code.
GO_PKGS=$(go list ./... 2>/dev/null | grep -v '/competitors/')
if ! go vet $GO_PKGS 2>&1; then
  fail "go vet"
  exit 1
fi
ok "go vet"

# ── Build ────────────────────────────────────────────────────────────

section "Build"

if ! go build -o seaportal ./cmd/seaportal 2>&1; then
  fail "go build"
  exit 1
fi
ok "go build"

# ── Lint ─────────────────────────────────────────────────────────────

section "Lint"

LINT_CMD=""
if command -v golangci-lint >/dev/null 2>&1; then
  LINT_CMD="golangci-lint"
elif [ -x "$HOME/bin/golangci-lint" ]; then
  LINT_CMD="$HOME/bin/golangci-lint"
elif [ -x "/usr/local/bin/golangci-lint" ]; then
  LINT_CMD="/usr/local/bin/golangci-lint"
fi

if [ -n "$LINT_CMD" ]; then
  if ! $LINT_CMD run ./...; then
    fail "golangci-lint"
    exit 1
  fi
  ok "golangci-lint"
else
  echo -e "  ${ACCENT}·${NC} golangci-lint not installed — skipping"
  hint "Install: brew install golangci-lint"
fi

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
echo ""
echo -e "  ${SUCCESS}${BOLD}All checks passed!${NC} Ready to push."
echo ""
