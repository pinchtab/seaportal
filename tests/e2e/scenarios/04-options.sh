#!/bin/bash
# 04-options.sh — CLI options tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "--version shows version"

if sp_ok --version; then
  if assert_output_contains "seaportal" "shows seaportal"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "-v shows version"

if sp_ok -v; then
  if assert_output_contains "seaportal" "shows seaportal"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--help shows usage"

sp --help
if assert_output_contains "Usage" "shows usage" && \
   assert_output_contains "Options" "shows options"; then
  pass_test
else
  fail_test
fi

# ─────────────────────────────────────────────────────────────────
start_test "no args shows usage and exits non-zero"

if sp_fail; then
  if assert_output_contains "Usage" "shows usage on no args"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "should exit non-zero with no args"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--fast option works"

if sp_ok --fast "$FIXTURES_URL/article.html"; then
  pass_test
else
  fail_test "command failed with --fast"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--no-dedupe option works"

if sp_ok --no-dedupe "$FIXTURES_URL/article.html"; then
  pass_test
else
  fail_test "command failed with --no-dedupe"
fi

# ─────────────────────────────────────────────────────────────────
start_test "combined --json --fast works"

if sp_ok --json --fast "$FIXTURES_URL/article.html"; then
  if assert_json_field_exists "url"; then
    pass_test
  else
    fail_test "JSON output malformed"
  fi
else
  fail_test "command failed"
fi
