#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

ok()      { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail()    { echo -e "  ${ERROR}✗${NC} $1"; ERRORS=$((ERRORS + 1)); }
warn()    { echo -e "  ${ACCENT}·${NC} $1"; WARNINGS=$((WARNINGS + 1)); }
hint()    { echo -e "    ${MUTED}$1${NC}"; }

ERRORS=0
WARNINGS=0

echo ""
echo -e "  ${ACCENT}${BOLD}🏥 SeaPortal Doctor${NC}"
echo -e "  ${MUTED}Checking development environment...${NC}"
echo ""

# ── Go ───────────────────────────────────────────────────────────────

echo -e "  ${BOLD}Go${NC}"

if command -v go &>/dev/null; then
  GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
  REQUIRED=$(grep '^go ' go.mod | awk '{print $2}')
  ok "go $GO_VERSION (required: $REQUIRED)"
else
  fail "go not installed"
  hint "Install: https://go.dev/dl/ or brew install go"
fi

# ── golangci-lint ────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Linting${NC}"

if command -v golangci-lint &>/dev/null; then
  LINT_VERSION=$(golangci-lint version 2>&1 | head -1)
  ok "golangci-lint ($LINT_VERSION)"
else
  warn "golangci-lint not installed (optional but recommended)"
  hint "Install: brew install golangci-lint"
fi

# ── Git hooks ────────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Git Hooks${NC}"

if [ -x .git/hooks/pre-commit ]; then
  ok "pre-commit hook installed"
else
  warn "pre-commit hook not installed"
  hint "Run: bash scripts/install-hooks.sh"
fi

# ── Dependencies ─────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Dependencies${NC}"

if go mod download 2>&1; then
  ok "go mod download"
else
  fail "go mod download failed"
fi

# ── Build ────────────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Build${NC}"

if go build -o /dev/null ./cmd/seaportal 2>&1; then
  ok "go build"
else
  fail "go build failed"
fi

# ── Tests ────────────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Tests${NC}"

if go test ./... -count=1 -short 2>&1 | tail -1 | grep -q "^ok\|PASS"; then
  ok "go test (short)"
else
  # Still try to show pass — go test exits 0 on success
  if go test ./... -count=1 -short &>/dev/null; then
    ok "go test (short)"
  else
    fail "go test failed"
  fi
fi

# ── Docker (optional, for e2e) ───────────────────────────────────────

echo ""
echo -e "  ${BOLD}Docker (optional)${NC}"

if command -v docker &>/dev/null; then
  ok "docker installed"
  if docker compose version &>/dev/null; then
    ok "docker compose available"
  else
    warn "docker compose not available (needed for e2e tests)"
  fi
else
  warn "docker not installed (needed for e2e tests only)"
  hint "Install: https://docs.docker.com/get-docker/"
fi

# ── Summary ──────────────────────────────────────────────────────────

echo ""
echo -e "  ${BOLD}Summary${NC}"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
  echo -e "  ${SUCCESS}${BOLD}All good!${NC} Environment is ready."
elif [ $ERRORS -eq 0 ]; then
  echo -e "  ${SUCCESS}${BOLD}Ready${NC} with ${ACCENT}$WARNINGS warning(s)${NC}"
else
  echo -e "  ${ERROR}${BOLD}$ERRORS critical issue(s)${NC}, ${ACCENT}$WARNINGS warning(s)${NC}"
fi
echo ""
