#!/bin/bash
# 03-detection.sh — SPA and blocked page detection

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "detect SPA page needs browser"

if sp_ok --json "$FIXTURES_URL/spa.html"; then
  is_spa=$(echo "$SP_OUT" | jq -r '.isSpa // false')
  needs_browser=$(echo "$SP_OUT" | jq -r '.validation.needsBrowser // false')
  
  if [ "$is_spa" = "true" ] || [ "$needs_browser" = "true" ]; then
    echo -e "    ${GREEN}✓${NC} detected (IsSPA=$is_spa, NeedsBrowser=$needs_browser)"
    pass_test
  else
    fail_test "should detect as SPA or needing browser"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "detect blocked/challenge page"

if sp_ok --json "$FIXTURES_URL/blocked.html"; then
  is_blocked=$(echo "$SP_OUT" | jq -r '.isBlocked // false')
  needs_browser=$(echo "$SP_OUT" | jq -r '.validation.needsBrowser // false')
  page_class=$(echo "$SP_OUT" | jq -r '.profile.class // ""')
  
  if [ "$is_blocked" = "true" ] || [ "$needs_browser" = "true" ] || [ "$page_class" = "blocked" ]; then
    echo -e "    ${GREEN}✓${NC} detected (IsBlocked=$is_blocked, class=$page_class)"
    pass_test
  else
    fail_test "should detect as blocked (IsBlocked=$is_blocked, class=$page_class)"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "article page classified correctly"

if sp_ok --json "$FIXTURES_URL/article.html"; then
  page_class=$(echo "$SP_OUT" | jq -r '.profile.class // ""')
  is_spa=$(echo "$SP_OUT" | jq -r '.isSpa // false')
  
  if [ "$is_spa" = "false" ] && [ "$page_class" != "spa" ]; then
    echo -e "    ${GREEN}✓${NC} not SPA (class=$page_class)"
    pass_test
  else
    fail_test "article should not be SPA (IsSPA=$is_spa, class=$page_class)"
  fi
else
  fail_test "command failed"
fi
