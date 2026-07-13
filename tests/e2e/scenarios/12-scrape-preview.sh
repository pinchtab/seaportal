#!/bin/bash
# 12-scrape-preview.sh — `scrape --preview` maps the site tree cheaply: one
# representative sample per URL pattern, with per-group counts, so a caller can
# decide which branch to expand.

source "$(dirname "$0")/common.sh"

require_host "$SCRAPE_SITE_URL/robots.txt" || return 0

# Direct invocation (not sp_ok): --allow-internal must follow the subcommand.
preview_json() {
  SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --allow-internal --preview --output json 2>&1)
  SP_EXIT=$?
}

# ─────────────────────────────────────────────────────────────────
start_test "preview: 1 sample per pattern, counts preserved"
preview_json
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
elif ! echo "$SP_OUT" | jq empty >/dev/null 2>&1; then
  fail_test "invalid JSON"
else
  maxg=$(echo "$SP_OUT" | jq -r '[.pageGroups[].sampled] | max')
  groups=$(echo "$SP_OUT" | jq -r '.pageGroups | length')
  # A group with more URLs in the sitemap than it sampled proves the counts
  # are real (the "tree" the user reads to decide what to expand).
  hascount=$(echo "$SP_OUT" | jq -e '[.pageGroups[] | select(.totalInSitemap > .sampled)] | length > 0' >/dev/null && echo yes || echo no)
  if [ "${maxg:-999}" -le 1 ] && [ "${groups:-0}" -ge 1 ] && [ "$hascount" = "yes" ]; then
    echo -e "    ${GREEN}✓${NC} $groups groups, max sampled=$maxg per pattern, counts present"; pass_test
  else
    fail_test "expected <=1 sample/group over >=1 groups with counts; got groups=$groups maxSampled=$maxg counts=$hascount"
  fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "preview: total sampled bounded by the group count"
if [ "$SP_EXIT" -eq 0 ]; then
  sampled=$(echo "$SP_OUT" | jq -r '.site.sampledPages')
  groups=$(echo "$SP_OUT" | jq -r '.pageGroups | length')
  if [ "${sampled:-999}" -le "${groups:-0}" ]; then
    echo -e "    ${GREEN}✓${NC} sampledPages=$sampled <= groups=$groups"; pass_test
  else
    fail_test "sampledPages=$sampled > groups=$groups (preview should take 1 per group)"
  fi
else
  fail_test "prior scrape failed"
fi
