#!/bin/bash
# 06-scrape-basic.sh — Basic site-scrape discovery against the multi-page
# fixture (ALP-014): sitemap discovery, pattern grouping, and populated pages.

source "$(dirname "$0")/common.sh"

SCRAPE_SITE_URL="${SCRAPE_SITE_URL:-http://scrapesite:80}"

# ─────────────────────────────────────────────────────────────────
start_test "scrape discovery: sitemap found, groups, counts"

# The scrape pipeline fetches the fixture URLs directly (no SSRF guard), so we
# invoke the subcommand straight instead of via sp_ok — sp_ok prepends
# --allow-internal *before* the subcommand, which would break dispatch.
SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output json 2>&1)
SP_EXIT=$?

if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "scrape exited $SP_EXIT: ${SP_OUT:0:200}"
else
  ok=1
  assert_json_field "site.sitemapFound" "true" "site.sitemapFound == true" || ok=0
  assert_json_field "site.totalURLsInSitemap" "356" "site.totalURLsInSitemap == 356" || ok=0
  assert_json_field_exists "summary.contentTypes" "summary.contentTypes present" || ok=0

  # Expected pattern groups with non-zero sampled counts.
  blog_sampled=$(echo "$SP_OUT" | jq -r '[.pageGroups[] | select(.pattern=="/blog/*") | .sampled] | first // 0')
  prod_sampled=$(echo "$SP_OUT" | jq -r '[.pageGroups[] | select(.pattern=="/products/*/detail.html") | .sampled] | first // 0')
  if [ "${blog_sampled:-0}" -gt 0 ]; then
    echo -e "    ${GREEN}✓${NC} /blog/* group sampled=$blog_sampled"
  else
    echo -e "    ${RED}✗${NC} /blog/* group missing or sampled==0"; ok=0
  fi
  if [ "${prod_sampled:-0}" -gt 0 ]; then
    echo -e "    ${GREEN}✓${NC} /products/*/detail.html group sampled=$prod_sampled"
  else
    echo -e "    ${RED}✗${NC} /products/*/detail.html group missing or sampled==0"; ok=0
  fi

  # Pages are populated: at least one has both a title and markdown.
  populated=$(echo "$SP_OUT" | jq -r '[.pages[] | select((.title|length)>0 and (.markdown|length)>0)] | length')
  if [ "${populated:-0}" -gt 0 ]; then
    echo -e "    ${GREEN}✓${NC} $populated pages populated (title + markdown)"
  else
    echo -e "    ${RED}✗${NC} no populated pages"; ok=0
  fi

  # robots.txt Disallow honored: no /private page in the results.
  if echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("/private"))] | length == 0' >/dev/null; then
    echo -e "    ${GREEN}✓${NC} robots-disallowed /private filtered out"
  else
    echo -e "    ${RED}✗${NC} /private page leaked into results"; ok=0
  fi

  if [ "$ok" -eq 1 ]; then pass_test; else fail_test; fi
fi
