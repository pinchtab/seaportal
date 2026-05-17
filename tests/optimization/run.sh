#!/usr/bin/env bash
# Runs `seaportal --json <url>` for every site in sites.tsv, asserts pageClass &
# content marker, and writes a results JSON + a Markdown summary.
#
# Usage:
#   ./run.sh                # run full suite
#   ./run.sh --category gov # only sites with category=gov
#   ./run.sh --jobs 8       # parallelism (default 4)
#   ./run.sh --timeout 30   # per-site seconds (default 30)
#
# Requires: seaportal on PATH, jq.

set -u
set -o pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SITES_FILE="$SCRIPT_DIR/sites.tsv"
RESULTS_DIR="$SCRIPT_DIR/results"
TIMESTAMP=$(date -u +%Y%m%d_%H%M%S)
RESULT_JSON="$RESULTS_DIR/run_${TIMESTAMP}.json"
RESULT_MD="$RESULTS_DIR/run_${TIMESTAMP}.md"

CATEGORY_FILTER=""
JOBS=4
TIMEOUT=30

while [[ $# -gt 0 ]]; do
  case "$1" in
    --category) CATEGORY_FILTER="$2"; shift 2 ;;
    --jobs)     JOBS="$2"; shift 2 ;;
    --timeout)  TIMEOUT="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

command -v seaportal >/dev/null || { echo "seaportal not on PATH" >&2; exit 1; }
command -v jq        >/dev/null || { echo "jq not on PATH" >&2; exit 1; }

# Portable timeout: prefer `timeout`, then `gtimeout` (brew coreutils), else no timeout.
if   command -v timeout  >/dev/null; then TIMEOUT_CMD="timeout"
elif command -v gtimeout >/dev/null; then TIMEOUT_CMD="gtimeout"
else TIMEOUT_CMD=""
fi

mkdir -p "$RESULTS_DIR"

probe_one() {
  local category="$1" url="$2" expect_class="$3" expect_marker="$4"
  local out
  local t0 t1 elapsed
  t0=$(date +%s)
  if [[ -n "$TIMEOUT_CMD" ]]; then
    out=$("$TIMEOUT_CMD" "${TIMEOUT}s" seaportal --json --fast "$url" 2>/dev/null)
  else
    out=$(seaportal --json --fast "$url" 2>/dev/null)
  fi
  if [[ -z "$out" ]]; then
    t1=$(date +%s); elapsed=$((t1 - t0))
    jq -nc \
      --arg category "$category" --arg url "$url" \
      --arg expect_class "$expect_class" --arg expect_marker "$expect_marker" \
      --argjson elapsed "$elapsed" \
      '{category:$category, url:$url, expect_class:$expect_class, expect_marker:$expect_marker,
        got_class:null, length:0, confidence:0, validation_ok:null, needs_browser:null,
        marker_found:false, class_match:false, pass:false, error:"fetch_failed_or_timeout",
        elapsed_seconds:$elapsed}'
    return
  fi
  t1=$(date +%s); elapsed=$((t1 - t0))

  local got_class length confidence validation_ok needs_browser content
  got_class=$(printf '%s' "$out"      | jq -r '.pageClass // .Profile.Class // .profile.class // ""')
  length=$(printf '%s' "$out"         | jq -r '.Length // .length // 0')
  confidence=$(printf '%s' "$out"     | jq -r '.Confidence // .confidence // 0')
  validation_ok=$(printf '%s' "$out"  | jq -r '.Validation.IsValid // .validation.isValid // null')
  needs_browser=$(printf '%s' "$out"  | jq -r '.Validation.NeedsBrowser // .validation.needsBrowser // null')
  # Concatenate body + title so markers can match either field — readability
  # extracts page titles into .title, not .content.
  content=$(printf '%s' "$out"        | jq -r '[.Content, .content, .Title, .title] | map(select(. != null and . != "")) | join("\n")')
  validation_json=$(printf '%s' "$out" | jq -c '.Validation // .validation // null')

  local marker_found=false class_match=false
  if [[ -z "$expect_marker" || "$expect_marker" == "-" ]]; then
    marker_found=true
  elif grep -qiF -- "$expect_marker" <<< "$content"; then
    # Here-string instead of `printf | grep` — the pipe form silently dropped
    # large $content payloads in backgrounded subshells.
    marker_found=true
  fi
  if [[ -z "$expect_class" || "$expect_class" == "any" || "$expect_class" == "$got_class" ]]; then
    class_match=true
  fi

  local pass=false
  if $marker_found && $class_match; then pass=true; fi

  jq -nc \
    --arg category "$category" --arg url "$url" \
    --arg expect_class "$expect_class" --arg expect_marker "$expect_marker" \
    --arg got_class "$got_class" \
    --argjson length "$length" --argjson confidence "$confidence" \
    --argjson elapsed "$elapsed" \
    --arg validation_ok "$validation_ok" --arg needs_browser "$needs_browser" \
    --argjson validation "${validation_json:-null}" \
    --argjson marker_found "$marker_found" --argjson class_match "$class_match" \
    --argjson pass "$pass" \
    '{category:$category, url:$url, expect_class:$expect_class, expect_marker:$expect_marker,
      got_class:$got_class, length:$length, confidence:$confidence,
      validation_ok:($validation_ok|fromjson? // null),
      needs_browser:($needs_browser|fromjson? // null),
      validation:$validation,
      marker_found:$marker_found, class_match:$class_match, pass:$pass,
      error:null, elapsed_seconds:$elapsed}'
}

export -f probe_one
export TIMEOUT TIMEOUT_CMD

# Stream results into a JSONL file, then assemble
JSONL="$RESULTS_DIR/run_${TIMESTAMP}.jsonl"
: > "$JSONL"

# Collect work into an array (category|url|class|marker, using $'\x1f' separator)
SEP=$'\x1f'
mapfile -t WORK < <(
  grep -v '^#' "$SITES_FILE" | grep -v '^[[:space:]]*$' \
  | awk -F'\t' -v cat="$CATEGORY_FILTER" -v sep="$SEP" '
      NF>=2 && (cat=="" || $1==cat) {
        ec = ($3=="" ? "any" : $3)
        em = ($4=="" ? "-"   : $4)
        printf "%s%s%s%s%s%s%s\n", $1, sep, $2, sep, ec, sep, em
      }'
)
total=${#WORK[@]}
echo "Running $total sites (jobs=$JOBS, timeout=${TIMEOUT}s)..."

# Simple job pool: spawn up to $JOBS background probes, wait when full
pids=()
for line in "${WORK[@]}"; do
  IFS="$SEP" read -r c u ec em <<< "$line"
  ( probe_one "$c" "$u" "$ec" "$em" >> "$JSONL" ) &
  pids+=("$!")
  if (( ${#pids[@]} >= JOBS )); then
    wait -n 2>/dev/null || wait "${pids[0]}"
    # prune finished
    new_pids=()
    for p in "${pids[@]}"; do kill -0 "$p" 2>/dev/null && new_pids+=("$p"); done
    pids=("${new_pids[@]}")
  fi
done
wait

# Assemble final JSON
jq -s '{
  timestamp: "'"$TIMESTAMP"'",
  total: length,
  passed: ([.[] | select(.pass)] | length),
  failed: ([.[] | select(.pass | not)] | length),
  by_category: (group_by(.category) | map({key:.[0].category, value:{total:length, passed:([.[]|select(.pass)]|length)}}) | from_entries),
  by_class: (group_by(.got_class) | map({key:(.[0].got_class // "none"), value:length}) | from_entries),
  results: .
}' "$JSONL" > "$RESULT_JSON"

passed=$(jq -r '.passed' "$RESULT_JSON")
failed=$(jq -r '.failed' "$RESULT_JSON")

{
  echo "# SeaPortal capability run — $TIMESTAMP"
  echo
  echo "**Total:** $total · **Passed:** $passed · **Failed:** $failed"
  echo
  echo "## Per-category"
  jq -r '.by_category | to_entries | sort_by(.key)
         | .[] | "- **\(.key)**: \(.value.passed)/\(.value.total)"' "$RESULT_JSON"
  echo
  echo "## Page-class distribution"
  jq -r '.by_class | to_entries | sort_by(-.value)
         | .[] | "- \(.key): \(.value)"' "$RESULT_JSON"
  echo
  echo "## Failures"
  jq -r '.results[] | select(.pass | not)
         | "- [\(.category)] \(.url) — got=\(.got_class // "—") expect=\(.expect_class) marker_found=\(.marker_found) error=\(.error // "")"' "$RESULT_JSON"
} > "$RESULT_MD"

echo
echo "Results:"
echo "  JSON: $RESULT_JSON"
echo "  MD:   $RESULT_MD"
echo "  Passed $passed / $total"
[[ "$failed" -eq 0 ]] || exit 1
