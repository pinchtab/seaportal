#!/bin/bash
# 01-basic.sh — Basic extraction tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "extract article page"

if sp_ok "$FIXTURES_URL/article.html"; then
  if assert_output_contains "Understanding Web Extraction" "extracts title" && \
     assert_output_contains "Web extraction is the process" "extracts content"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "extract minimal page"

if sp_ok "$FIXTURES_URL/minimal.html"; then
  if assert_output_contains "Hello World" "extracts h1"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi
