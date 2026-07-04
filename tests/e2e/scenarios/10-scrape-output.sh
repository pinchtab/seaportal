#!/bin/bash
# 10-scrape-output.sh — validate the three --output renderers (ALP-008)
# end-to-end against the fixture site.

source "$(dirname "$0")/common.sh"

SCRAPE_SITE_URL="${SCRAPE_SITE_URL:-http://scrapesite:80}"
OUT_DIR="$RESULTS_DIR/scrape-out"

# Invoked directly (not via sp_ok): the scrape path needs no SSRF flag and
# sp_ok's --allow-internal prefix would break subcommand dispatch.

# ─────────────────────────────────────────────────────────────────
start_test "output: --output json is well-formed with expected top-level keys"
SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output json --max-pages 20 2>&1)
if [ "$?" -ne 0 ]; then
  fail_test "scrape json failed: ${SP_OUT:0:160}"
elif echo "$SP_OUT" | jq -e 'has("site") and has("pageGroups") and has("pages") and has("summary")' >/dev/null 2>&1; then
  echo -e "    ${GREEN}✓${NC} valid JSON with site/pageGroups/pages/summary"; pass_test
else
  fail_test "missing top-level keys or invalid JSON"
fi

# ─────────────────────────────────────────────────────────────────
start_test "output: --output md has a title header and per-page sections"
SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output md --max-pages 20 2>&1)
if [ "$?" -ne 0 ]; then
  fail_test "scrape md failed"
else
  ok=1
  echo "$SP_OUT" | grep -qE '^# ' || { echo -e "    ${RED}✗${NC} no h1 title header"; ok=0; }
  echo "$SP_OUT" | grep -q '## Pages' || { echo -e "    ${RED}✗${NC} no Pages section"; ok=0; }
  echo "$SP_OUT" | grep -qE '^### ' || { echo -e "    ${RED}✗${NC} no per-page section"; ok=0; }
  echo "$SP_OUT" | grep -q '## Summary' || { echo -e "    ${RED}✗${NC} no Summary section"; ok=0; }
  if [ "$ok" -eq 1 ]; then
    echo -e "    ${GREEN}✓${NC} markdown has title + Pages/### sections + Summary"; pass_test
  else
    fail_test "markdown structure incomplete"
  fi
fi

# ─────────────────────────────────────────────────────────────────
start_test "output: --output directory writes result.json, pages/*.md, index.md"
rm -rf "$OUT_DIR"
SP_OUT=$(seaportal scrape "$SCRAPE_SITE_URL/" --output directory --out-dir "$OUT_DIR" --max-pages 20 2>&1)
if [ "$?" -ne 0 ]; then
  fail_test "scrape directory failed: ${SP_OUT:0:160}"
else
  ok=1
  [ -s "$OUT_DIR/result.json" ] || { echo -e "    ${RED}✗${NC} result.json missing/empty"; ok=0; }
  [ -s "$OUT_DIR/index.md" ] || { echo -e "    ${RED}✗${NC} index.md manifest missing/empty"; ok=0; }
  jq empty "$OUT_DIR/result.json" >/dev/null 2>&1 || { echo -e "    ${RED}✗${NC} result.json is not valid JSON"; ok=0; }

  nmd=$(find "$OUT_DIR/pages" -name '*.md' 2>/dev/null | wc -l | tr -d ' ')
  if [ "${nmd:-0}" -gt 0 ]; then
    echo -e "    ${GREEN}✓${NC} $nmd per-page markdown files"
  else
    echo -e "    ${RED}✗${NC} no per-page markdown files"; ok=0
  fi
  # Every page file is non-empty and filesystem-safe (lowercase, digits, dashes).
  while IFS= read -r f; do
    [ -s "$f" ] || { echo -e "    ${RED}✗${NC} empty page file $f"; ok=0; }
    base=$(basename "$f")
    echo "$base" | grep -qE '^[a-z0-9-]+\.md$' || { echo -e "    ${RED}✗${NC} unsafe filename: $base"; ok=0; }
  done < <(find "$OUT_DIR/pages" -name '*.md' 2>/dev/null)

  if [ "$ok" -eq 1 ]; then
    echo -e "    ${GREEN}✓${NC} directory output complete and filesystem-safe"; pass_test
  else
    fail_test "directory output incomplete"
  fi
fi
