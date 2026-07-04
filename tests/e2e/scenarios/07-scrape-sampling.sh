#!/bin/bash
# 07-scrape-sampling.sh — sampling strategies + cap enforcement against the
# large fixture groups (ALP-014).

source "$(dirname "$0")/common.sh"

SCRAPE_SITE_URL="${SCRAPE_SITE_URL:-http://scrapesite:80}"

# scrape_json <args...> → runs the scrape subcommand, sets SP_OUT/SP_EXIT.
# Invoked directly (not via sp_ok) because the scrape path needs no SSRF flag
# and sp_ok's --allow-internal prefix would break subcommand dispatch.
scrape_json() {
  SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output json "$@" 2>&1)
  SP_EXIT=$?
}

# ─────────────────────────────────────────────────────────────────
start_test "sampling: --max-pages caps total"
scrape_json --max-pages 10
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT: ${SP_OUT:0:160}"
else
  total=$(echo "$SP_OUT" | jq -r '.site.sampledPages')
  if [ "${total:-999}" -le 10 ]; then
    echo -e "    ${GREEN}✓${NC} sampledPages=$total <= 10"; pass_test
  else
    fail_test "sampledPages=$total > 10"
  fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "sampling: --max-per-pattern caps each group"
scrape_json --max-per-pattern 3
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
else
  maxg=$(echo "$SP_OUT" | jq -r '[.pageGroups[].sampled] | max')
  if [ "${maxg:-999}" -le 3 ]; then
    echo -e "    ${GREEN}✓${NC} max group sampled=$maxg <= 3"; pass_test
  else
    fail_test "a group sampled=$maxg > 3"
  fi
fi

# ─────────────────────────────────────────────────────────────────
for strat in random priority balanced; do
  start_test "sampling: --sample-strategy $strat runs and emits valid JSON"
  scrape_json --sample-strategy "$strat"
  if [ "$SP_EXIT" -ne 0 ]; then
    fail_test "exit $SP_EXIT"
  elif ! echo "$SP_OUT" | jq empty >/dev/null 2>&1; then
    fail_test "invalid JSON"
  elif [ "$strat" = "priority" ]; then
    if echo "$SP_OUT" | jq -e '[.pages[] | select(.url | test("/index.html$"))] | length > 0' >/dev/null; then
      echo -e "    ${GREEN}✓${NC} priority sampled the homepage"; pass_test
    else
      fail_test "priority did not sample the homepage"
    fi
  else
    pass_test
  fi
done

# ─────────────────────────────────────────────────────────────────
start_test "sampling: --full bypasses caps on an include-filtered subset"
# The products group has 50 sitemap entries; --full must return all of them
# (well above the default max-per-pattern of 8), scoped to products only.
scrape_json --full --include-patterns '/products/**'
if [ "$SP_EXIT" -ne 0 ]; then
  fail_test "exit $SP_EXIT"
else
  count=$(echo "$SP_OUT" | jq -r '.pages | length')
  only_products=$(echo "$SP_OUT" | jq -e '[.pages[] | select((.url | test("/products/")) | not)] | length == 0' >/dev/null && echo yes || echo no)
  if [ "${count:-0}" -eq 50 ] && [ "$only_products" = "yes" ]; then
    echo -e "    ${GREEN}✓${NC} --full returned all 50 product URLs, products only"; pass_test
  else
    fail_test "expected 50 product-only pages, got count=$count products_only=$only_products"
  fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "sampling: deterministic across two identical runs"
scrape_json --sample-strategy random --max-per-pattern 3
a=$(echo "$SP_OUT" | jq -rS '[.pages[].url] | sort')
scrape_json --sample-strategy random --max-per-pattern 3
b=$(echo "$SP_OUT" | jq -rS '[.pages[].url] | sort')
if [ "$a" = "$b" ] && [ -n "$a" ]; then
  echo -e "    ${GREEN}✓${NC} identical sampled URL set across runs"; pass_test
else
  fail_test "sampling not deterministic"
fi
