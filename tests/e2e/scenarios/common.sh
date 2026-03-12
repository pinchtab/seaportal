#!/bin/bash
# Common utilities for SeaPortal CLI E2E tests

set -uo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Environment
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"
RESULTS_DIR="${RESULTS_DIR:-/results}"

# Test tracking
TESTS_PASSED=0
TESTS_FAILED=0
CURRENT_TEST=""

# Last command output
SP_OUT=""
SP_EXIT=0

# ─────────────────────────────────────────────────────────────────
# Test lifecycle
# ─────────────────────────────────────────────────────────────────

start_test() {
  CURRENT_TEST="$1"
  echo -e "${BLUE}▶ ${CURRENT_TEST}${NC}"
}

pass_test() {
  echo -e "  ${GREEN}✓${NC} ${CURRENT_TEST}"
  ((TESTS_PASSED++)) || true
}

fail_test() {
  local reason="${1:-}"
  echo -e "  ${RED}✗${NC} ${CURRENT_TEST}"
  [ -n "$reason" ] && echo -e "    ${RED}→ ${reason}${NC}"
  ((TESTS_FAILED++)) || true
}

# ─────────────────────────────────────────────────────────────────
# CLI execution helpers
# ─────────────────────────────────────────────────────────────────

sp() {
  SP_OUT=$(seaportal "$@" 2>&1) || SP_EXIT=$?
  SP_EXIT=${SP_EXIT:-0}
}

sp_ok() {
  SP_EXIT=0
  SP_OUT=$(seaportal "$@" 2>&1) || SP_EXIT=$?
  if [ "$SP_EXIT" -ne 0 ]; then
    fail_test "expected exit 0, got $SP_EXIT"
    echo "    stdout: $SP_OUT"
    return 1
  fi
  return 0
}

sp_fail() {
  SP_EXIT=0
  SP_OUT=$(seaportal "$@" 2>&1) || SP_EXIT=$?
  if [ "$SP_EXIT" -eq 0 ]; then
    fail_test "expected non-zero exit, got 0"
    return 1
  fi
  return 0
}

# ─────────────────────────────────────────────────────────────────
# Assertions
# ─────────────────────────────────────────────────────────────────

assert_output_contains() {
  local needle="$1"
  local desc="${2:-contains '$needle'}"
  if echo "$SP_OUT" | grep -q "$needle"; then
    echo -e "    ${GREEN}✓${NC} $desc"
    return 0
  else
    echo -e "    ${RED}✗${NC} $desc"
    echo "    output: ${SP_OUT:0:200}..."
    return 1
  fi
}

assert_output_not_contains() {
  local needle="$1"
  local desc="${2:-does not contain '$needle'}"
  if echo "$SP_OUT" | grep -q "$needle"; then
    echo -e "    ${RED}✗${NC} $desc"
    return 1
  else
    echo -e "    ${GREEN}✓${NC} $desc"
    return 0
  fi
}

assert_json_field() {
  local field="$1"
  local expected="$2"
  local desc="${3:-$field equals $expected}"
  local actual
  actual=$(echo "$SP_OUT" | jq -r ".$field" 2>/dev/null || echo "")
  if [ "$actual" = "$expected" ]; then
    echo -e "    ${GREEN}✓${NC} $desc"
    return 0
  else
    echo -e "    ${RED}✗${NC} $desc (got: $actual)"
    return 1
  fi
}

assert_json_field_exists() {
  local field="$1"
  local desc="${2:-$field exists}"
  local val
  val=$(echo "$SP_OUT" | jq -r ".$field // empty" 2>/dev/null || echo "")
  if [ -n "$val" ]; then
    echo -e "    ${GREEN}✓${NC} $desc"
    return 0
  else
    echo -e "    ${RED}✗${NC} $desc"
    return 1
  fi
}

# ─────────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────────

print_summary() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Results"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  echo -e "  Passed: ${GREEN}${TESTS_PASSED}${NC}"
  echo -e "  Failed: ${RED}${TESTS_FAILED}${NC}"
  echo ""

  # Write results file
  cat > "$RESULTS_DIR/summary.json" <<EOF
{
  "passed": $TESTS_PASSED,
  "failed": $TESTS_FAILED,
  "total": $((TESTS_PASSED + TESTS_FAILED))
}
EOF

  if [ "$TESTS_FAILED" -gt 0 ]; then
    echo -e "  ${RED}FAILED${NC}"
    exit 1
  else
    echo -e "  ${GREEN}ALL PASSED${NC}"
    exit 0
  fi
}
