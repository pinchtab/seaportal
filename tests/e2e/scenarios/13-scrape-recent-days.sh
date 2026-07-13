#!/bin/bash
# 13-scrape-recent-days.sh — `scrape --recent-days N` bounds sitemap discovery
# by <lastmod>. The datedsite fixture's sitemap holds future-dated "fresh"
# pages (always within any window) and ancient "stale" pages (always outside),
# so the split is stable regardless of when the test runs.

source "$(dirname "$0")/common.sh"

require_host "$DATED_SITE_URL/robots.txt" || return 0

dated_json() {
  SP_OUT=$(seaportal scrape "$DATED_SITE_URL/" --allow-internal --output json "$@" 2>&1)
  SP_EXIT=$?
}

# ─────────────────────────────────────────────────────────────────
start_test "recent-days: no filter discovers both fresh and stale"
dated_json --full
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
else
  fresh=$(echo "$SP_OUT" | jq -r '[.pages[] | select(.url | test("/fresh-"))] | length')
  stale=$(echo "$SP_OUT" | jq -r '[.pages[] | select(.url | test("/stale-"))] | length')
  if [ "${fresh:-0}" -ge 1 ] && [ "${stale:-0}" -ge 1 ]; then
    echo -e "    ${GREEN}✓${NC} baseline discovered fresh=$fresh stale=$stale"; pass_test
  else
    fail_test "expected both fresh and stale without filter; got fresh=$fresh stale=$stale"
  fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "recent-days: --recent-days 30 keeps fresh, drops stale"
dated_json --full --recent-days 30
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
else
  fresh=$(echo "$SP_OUT" | jq -r '[.pages[] | select(.url | test("/fresh-"))] | length')
  stale=$(echo "$SP_OUT" | jq -r '[.pages[] | select(.url | test("/stale-"))] | length')
  if [ "${fresh:-0}" -ge 1 ] && [ "${stale:-0}" -eq 0 ]; then
    echo -e "    ${GREEN}✓${NC} recent-days kept fresh=$fresh, dropped all stale"; pass_test
  else
    fail_test "expected fresh kept and stale dropped; got fresh=$fresh stale=$stale"
  fi
fi
