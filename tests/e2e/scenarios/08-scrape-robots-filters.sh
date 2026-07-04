#!/bin/bash
# 08-scrape-robots-filters.sh — robots.txt respect + include/exclude pattern
# selection against the fixture site (which Disallows /private per ALP-014).

source "$(dirname "$0")/common.sh"

require_host "$SCRAPE_SITE_URL/robots.txt" || return 0

# Invoked directly (not via sp_ok): the scrape path needs no SSRF flag and
# sp_ok's --allow-internal prefix would break subcommand dispatch.
scrape_json() {
  SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output json "$@" 2>&1)
  SP_EXIT=$?
}

# ─────────────────────────────────────────────────────────────────
start_test "robots: disallowed path filtered by default"
scrape_json
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
elif echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("/private"))] | length == 0' >/dev/null; then
  echo -e "    ${GREEN}✓${NC} /private absent with --respect-robots (default)"; pass_test
else
  fail_test "/private leaked despite robots respect"
fi

# ─────────────────────────────────────────────────────────────────
start_test "robots: --respect-robots=false exposes disallowed path (flag wired)"
scrape_json --respect-robots=false
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
elif echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("/private"))] | length > 0' >/dev/null; then
  echo -e "    ${GREEN}✓${NC} /private present when robots respect disabled"; pass_test
else
  fail_test "/private still absent — flag not wired"
fi

# ─────────────────────────────────────────────────────────────────
start_test "filters: --exclude-patterns removes products"
# Product URLs are two segments deep (/products/<id>/detail.html), so the
# cross-segment ** glob is required to match them.
scrape_json --exclude-patterns '/products/**'
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
elif echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("/products/"))] | length == 0' >/dev/null; then
  echo -e "    ${GREEN}✓${NC} no product URLs after exclude"; pass_test
else
  fail_test "product URLs present despite exclude"
fi

# ─────────────────────────────────────────────────────────────────
start_test "filters: --include-patterns keeps only blog"
scrape_json --include-patterns '/blog/*'
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
else
  total=$(echo "$SP_OUT" | jq -r '.pages | length')
  nonblog=$(echo "$SP_OUT" | jq -r '[.pages[] | select((.url | test("/blog/")) | not)] | length')
  if [ "${total:-0}" -gt 0 ] && [ "${nonblog:-1}" -eq 0 ]; then
    echo -e "    ${GREEN}✓${NC} $total pages, all under /blog/"; pass_test
  else
    fail_test "expected only blog URLs (total=$total non-blog=$nonblog)"
  fi
fi
