#!/usr/bin/env bash
# Append a JSON line for one task step.
# Usage: ./record.sh <step_id> <outcome> <note>
#   step_id : e.g. "1.3"
#   outcome : pass | fail | escalate | escalate-paywall
#   note    : one-line observation (will be JSON-escaped)
#
# `escalate` and `escalate-paywall` are both first-class success outcomes —
# they mean seaportal correctly recognised a content limit. Use plain
# `escalate` when nothing usable came back (SPA, bot-block, captcha,
# `needsBrowser=true`). Use `escalate-paywall` when seaportal extracted a
# preview (abstract, headline) but the substantive body is gated behind a
# subscription or partial-content login wall (e.g. Repubblica articles, WSJ).
#
# Honors $SEAPORTAL_REPORT_FILE if set, else writes to results/agent_<ts>.jsonl
set -u

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
[[ -z "${SEAPORTAL_REPORT_FILE:-}" ]] && {
  ts=$(date -u +%Y%m%d_%H%M%S)
  mkdir -p "$SCRIPT_DIR/results"
  export SEAPORTAL_REPORT_FILE="$SCRIPT_DIR/results/agent_${ts}.jsonl"
}

step="${1:?step id required, e.g. 1.3}"
outcome="${2:?outcome required: pass|fail|escalate|escalate-paywall}"
note="${3:-}"

case "$outcome" in
  pass|fail|escalate|escalate-paywall) ;;
  *) echo "outcome must be pass|fail|escalate|escalate-paywall (got: $outcome)" >&2; exit 2 ;;
esac

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
jq -nc \
  --arg ts "$ts" --arg step "$step" --arg outcome "$outcome" --arg note "$note" \
  '{ts:$ts, step:$step, outcome:$outcome, note:$note}' \
  >> "$SEAPORTAL_REPORT_FILE"

echo "recorded: $step → $outcome → $SEAPORTAL_REPORT_FILE"
