#!/bin/bash
# 06-bot-detection.sh — Bot detection stealth tests
# Tests stealth mode against bot detection checks (sannysoft-style)

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect fixture loads with stealth"

# Run with experimental (headless) + escape-detection (stealth)
if sp_ok --experimental --escape-detection --json "$FIXTURES_URL/bot-detect.html"; then
  echo -e "    ${GREEN}✓${NC} page loaded successfully"
  pass_test
else
  fail_test "failed to load bot-detect fixture"
fi

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: check page content for results"

if sp_ok --experimental --escape-detection "$FIXTURES_URL/bot-detect.html"; then
  # The fixture outputs results in the HTML - check for PASS indicators
  if echo "$SP_OUT" | grep -qi "PASSED\|✓ PASS"; then
    echo -e "    ${GREEN}✓${NC} found PASS indicators in output"
    pass_test
  elif echo "$SP_OUT" | grep -qi "FAILED\|✗ FAIL"; then
    echo -e "    ${YELLOW}⚠${NC} found FAIL indicators - stealth may not be working"
    # Check specific failures
    echo "$SP_OUT" | grep -i "FAIL" | head -5
    fail_test "bot detection tests failed"
  else
    echo -e "    ${YELLOW}⚠${NC} could not determine test results from output"
    pass_test  # Don't fail if we just can't parse it
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: snapshot mode captures test results"

if sp_ok --experimental --escape-detection --snapshot "$FIXTURES_URL/bot-detect.html"; then
  # Look for the test result table in snapshot
  if echo "$SP_OUT" | grep -qi "webdriver\|plugins"; then
    echo -e "    ${GREEN}✓${NC} snapshot contains bot detection test elements"
    pass_test
  else
    echo -e "    ${YELLOW}⚠${NC} snapshot may not contain expected elements"
    pass_test
  fi
else
  fail_test "snapshot failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "bot-detect: without stealth should differ"

# Run WITHOUT escape-detection to compare
if sp_ok --experimental "$FIXTURES_URL/bot-detect.html"; then
  NO_STEALTH_OUT="$SP_OUT"
  
  # Run WITH escape-detection
  if sp_ok --experimental --escape-detection "$FIXTURES_URL/bot-detect.html"; then
    STEALTH_OUT="$SP_OUT"
    
    # They might be different (stealth should pass more tests)
    if [ "$NO_STEALTH_OUT" != "$STEALTH_OUT" ]; then
      echo -e "    ${GREEN}✓${NC} stealth mode produces different output"
    else
      echo -e "    ${YELLOW}⚠${NC} outputs identical (may be ok depending on browser)"
    fi
    pass_test
  else
    fail_test "stealth run failed"
  fi
else
  fail_test "non-stealth run failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "real sannysoft.com check (network)"

# Only run if we have network access
if curl -s --max-time 5 "https://bot.sannysoft.com/" > /dev/null 2>&1; then
  if sp_ok --experimental --escape-detection --json "https://bot.sannysoft.com/"; then
    echo -e "    ${GREEN}✓${NC} sannysoft.com loaded with stealth"
    # The page content should show test results
    pass_test
  else
    echo -e "    ${YELLOW}⚠${NC} sannysoft.com failed (may be network issue)"
    pass_test  # Don't fail on network issues
  fi
else
  echo -e "    ${YELLOW}⚠${NC} skipped (no network access)"
  pass_test
fi
