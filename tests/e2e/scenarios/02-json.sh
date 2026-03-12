#!/bin/bash
# 02-json.sh — JSON output tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "JSON output has required fields"

if sp_ok --json "$FIXTURES_URL/article.html"; then
  if assert_json_field_exists "url" && \
     assert_json_field_exists "title" && \
     assert_json_field_exists "content" && \
     assert_json_field_exists "length" && \
     assert_json_field_exists "confidence"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "JSON output has classification"

if sp_ok --json "$FIXTURES_URL/article.html"; then
  if assert_json_field_exists "profile" && \
     assert_json_field_exists "validation"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "JSON confidence is reasonable for article"

if sp_ok --json "$FIXTURES_URL/article.html"; then
  confidence=$(echo "$SP_OUT" | jq -r '.confidence // 0')
  if [ "$confidence" -ge 30 ]; then
    echo -e "    ${GREEN}✓${NC} confidence >= 30 (got $confidence)"
    pass_test
  else
    fail_test "confidence too low: $confidence"
  fi
else
  fail_test "command failed"
fi
