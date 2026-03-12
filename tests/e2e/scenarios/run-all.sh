#!/bin/bash
# Run all SeaPortal CLI E2E tests

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

source "$SCRIPT_DIR/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🌊 SeaPortal CLI E2E Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Fixtures: $FIXTURES_URL"
echo ""

# Verify seaportal CLI is available
if ! command -v seaportal &> /dev/null; then
  echo "ERROR: seaportal CLI not found in PATH"
  exit 1
fi

# Show version
echo "  Version: $(seaportal --version)"
echo ""

# Wait for fixtures server
echo "Waiting for fixtures server..."
for i in {1..30}; do
  if curl -sf "$FIXTURES_URL/" > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} fixtures server ready"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo -e "  ${RED}✗${NC} fixtures server not ready"
    exit 1
  fi
  sleep 1
done

echo ""
echo "Running tests..."
echo ""

# Run all test scripts in order
for script in "$SCRIPT_DIR"/[0-9][0-9]-*.sh; do
  if [ -f "$script" ]; then
    echo -e "${YELLOW}═══ $(basename "$script") ═══${NC}"
    echo ""
    source "$script"
    echo ""
  fi
done

print_summary
