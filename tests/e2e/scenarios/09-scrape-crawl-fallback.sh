#!/bin/bash
# 09-scrape-crawl-fallback.sh — no-sitemap discovery path: scrape must fall back
# to a bounded homepage crawl (ALP-002) against the site-nosm fixture.

source "$(dirname "$0")/common.sh"

require_host "$CRAWL_SITE_URL/index.html" || return 0

# Invoked directly (not via sp_ok): the scrape path needs no SSRF flag and
# sp_ok's --allow-internal prefix would break subcommand dispatch.
scrape_json() {
  SP_OUT=$(seaportal scrape "$CRAWL_SITE_URL/" --output json "$@" 2>&1)
  SP_EXIT=$?
}

# ─────────────────────────────────────────────────────────────────
start_test "crawl fallback: no sitemap → pages discovered by crawl"
scrape_json
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
else
  ok=1
  assert_json_field "site.sitemapFound" "false" "site.sitemapFound == false" || ok=0

  npages=$(echo "$SP_OUT" | jq -r '.pages | length')
  if [ "${npages:-0}" -gt 0 ]; then
    echo -e "    ${GREEN}✓${NC} $npages pages discovered via crawl"
  else
    echo -e "    ${RED}✗${NC} no pages discovered"; ok=0
  fi

  # Homepage (host root) is included.
  if echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("://[^/]+/$"))] | length > 0' >/dev/null; then
    echo -e "    ${GREEN}✓${NC} homepage present"
  else
    echo -e "    ${RED}✗${NC} homepage missing"; ok=0
  fi

  # Same-host only: no external URLs leaked into results.
  if echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("external.example"))] | length == 0' >/dev/null; then
    echo -e "    ${GREEN}✓${NC} crawl stayed same-host (no external URLs)"
  else
    echo -e "    ${RED}✗${NC} external URL leaked into results"; ok=0
  fi

  if [ "$ok" -eq 1 ]; then pass_test; else fail_test; fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "crawl fallback: respects the page budget"
scrape_json --max-pages 2
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
else
  n=$(echo "$SP_OUT" | jq -r '.pages | length')
  if [ "${n:-999}" -le 2 ]; then
    echo -e "    ${GREEN}✓${NC} crawl produced $n pages (<= --max-pages 2)"; pass_test
  else
    fail_test "crawl produced $n pages > budget 2"
  fi
fi
