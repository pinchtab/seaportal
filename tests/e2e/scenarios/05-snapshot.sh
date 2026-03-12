#!/bin/bash
# 05-snapshot.sh — Accessibility snapshot tests

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "--snapshot outputs JSON tree"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if assert_json_field "role" "document" "root is document" && \
     assert_json_field_exists "children" "has children"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot includes tag field"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.children[].tag' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "tag field not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot includes depth field on nested elements"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  # Check for depth > 0 on nested elements (depth=0 is omitted by omitempty)
  if echo "$SP_OUT" | jq -e '.. | objects | select(.depth > 0)' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "depth field not found on nested elements"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot includes selector field"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.children[].selector' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "selector field not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot includes ref field"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.children[].ref' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "ref field not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--filter=interactive shows only interactive elements"

if sp_ok --snapshot --filter=interactive "$FIXTURES_URL/snapshot-test.html"; then
  # Should have button and link (interactive)
  if echo "$SP_OUT" | jq -e '.. | objects | select(.role == "button")' >/dev/null 2>&1 && \
     echo "$SP_OUT" | jq -e '.. | objects | select(.role == "link")' >/dev/null 2>&1; then
    # Should NOT have standalone paragraph (non-interactive elements filtered out)
    if echo "$SP_OUT" | jq -e '.. | objects | select(.role == "paragraph")' >/dev/null 2>&1; then
      fail_test "non-interactive paragraph should be filtered"
    else
      pass_test
    fi
  else
    fail_test "interactive elements not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--format=compact outputs text tree"

if sp_ok --snapshot --format=compact "$FIXTURES_URL/snapshot-test.html"; then
  if assert_output_contains "document" "contains document" && \
     assert_output_not_contains "{" "not JSON"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "compact format includes tag names"

if sp_ok --snapshot --format=compact "$FIXTURES_URL/snapshot-test.html"; then
  if assert_output_contains "<nav>" "contains nav tag" || \
     assert_output_contains "<main>" "contains main tag"; then
    pass_test
  else
    fail_test "tag names not found in compact output"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "compact format shows interactive marker"

if sp_ok --snapshot --format=compact "$FIXTURES_URL/snapshot-test.html"; then
  if assert_output_contains "[interactive]" "shows interactive marker"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "--max-tokens limits output"

if sp_ok --snapshot --max-tokens=100 "$FIXTURES_URL/large-page.html"; then
  # Output should be smaller than full snapshot
  limited_size=${#SP_OUT}
  
  sp_ok --snapshot "$FIXTURES_URL/large-page.html"
  full_size=${#SP_OUT}
  
  if [ "$limited_size" -lt "$full_size" ]; then
    echo -e "    ${GREEN}✓${NC} limited output ($limited_size) < full ($full_size)"
    pass_test
  else
    fail_test "output not limited: $limited_size vs $full_size"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot handles headings with levels"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.. | objects | select(.role == "heading" and .level > 0)' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "heading with level not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "snapshot handles links with href"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.. | objects | select(.role == "link" and .href)' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "link with href not found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "selector uses id when available"

if sp_ok --snapshot "$FIXTURES_URL/snapshot-test.html"; then
  if echo "$SP_OUT" | jq -e '.. | objects | select(.selector and (.selector | startswith("#")))' >/dev/null 2>&1; then
    pass_test
  else
    fail_test "no id-based selector found"
  fi
else
  fail_test "command failed"
fi

# ─────────────────────────────────────────────────────────────────
start_test "combined --filter=interactive --format=compact"

if sp_ok --snapshot --filter=interactive --format=compact "$FIXTURES_URL/snapshot-test.html"; then
  if assert_output_contains "[interactive]" "shows interactive" && \
     assert_output_not_contains "{" "not JSON"; then
    pass_test
  else
    fail_test
  fi
else
  fail_test "command failed"
fi
