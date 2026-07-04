#!/bin/bash
# 11-scrape-horizontal.sh — the core "doesn't explode" property: a horizontal
# site (hundreds of /blog/* URLs) is grouped + sampled to a bounded fetch count
# and the run stays fast.

source "$(dirname "$0")/common.sh"

SCRAPE_SITE_URL="${SCRAPE_SITE_URL:-http://scrapesite:80}"
TIME_BUDGET_SECONDS="${SCRAPE_TIME_BUDGET:-30}"

# ─────────────────────────────────────────────────────────────────
start_test "horizontal: large sitemap sampled to a bounded, fast fetch"

# Default caps (max-pages 50, max-per-pattern 8). Invoked directly (not via
# sp_ok): the scrape path needs no SSRF flag and sp_ok's --allow-internal prefix
# would break subcommand dispatch.
t0=$(date +%s)
SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output json 2>&1)
rc=$?
t1=$(date +%s)
elapsed=$((t1 - t0))

if [ "$rc" -ne 0 ]; then
  fail_test "scrape exited $rc: ${SP_OUT:0:160}"
else
  ok=1

  total=$(echo "$SP_OUT" | jq -r '.site.totalURLsInSitemap')
  if [ "${total:-0}" -ge 300 ]; then
    echo -e "    ${GREEN}✓${NC} large sitemap: totalURLsInSitemap=$total"
  else
    echo -e "    ${RED}✗${NC} sitemap not large (totalURLsInSitemap=$total)"; ok=0
  fi

  sampled=$(echo "$SP_OUT" | jq -r '.site.sampledPages')
  if [ "${sampled:-999}" -le 50 ]; then
    echo -e "    ${GREEN}✓${NC} sampledPages=$sampled within --max-pages 50"
  else
    echo -e "    ${RED}✗${NC} sampledPages=$sampled exceeds 50 (exploded)"; ok=0
  fi

  # The large /blog/* group is a bounded sample, not every URL.
  blog_total=$(echo "$SP_OUT" | jq -r '[.pageGroups[] | select(.pattern=="/blog/*") | .totalInSitemap] | first // 0')
  blog_sampled=$(echo "$SP_OUT" | jq -r '[.pageGroups[] | select(.pattern=="/blog/*") | .sampled] | first // 0')
  if [ "${blog_total:-0}" -ge 300 ] && [ "${blog_sampled:-0}" -gt 0 ] && [ "${blog_sampled:-0}" -le 8 ]; then
    echo -e "    ${GREEN}✓${NC} /blog/* sampled $blog_sampled of $blog_total (bounded)"
  else
    echo -e "    ${RED}✗${NC} /blog/* not bounded (sampled=$blog_sampled total=$blog_total)"; ok=0
  fi

  # No fetch explosion: run completes under the wall-clock budget.
  if [ "$elapsed" -le "$TIME_BUDGET_SECONDS" ]; then
    echo -e "    ${GREEN}✓${NC} completed in ${elapsed}s (<= ${TIME_BUDGET_SECONDS}s budget)"
  else
    echo -e "    ${RED}✗${NC} took ${elapsed}s (> ${TIME_BUDGET_SECONDS}s budget)"; ok=0
  fi

  if [ "$ok" -eq 1 ]; then pass_test; else fail_test; fi
fi
