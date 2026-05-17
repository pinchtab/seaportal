#!/usr/bin/env bash
# selftest.sh — spawn ONE subagent against group-selftest.md and record
# outcomes via record.sh as JSONL. Designed to be invoked by
# `seabench selftest`. If no subagent runtime is available (i.e. this is
# running outside Claude Code or any equivalent agent harness), exit 0
# with a clear message so callers can fall back to --input replay.
#
# Honours $SEAPORTAL_REPORT_FILE if pre-set by the caller (e.g. seabench).
set -u
set -o pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
GROUP_FILE="$SCRIPT_DIR/group-selftest.md"

if [[ ! -f "$GROUP_FILE" ]]; then
  echo "selftest: missing $GROUP_FILE" >&2
  exit 2
fi

if [[ -z "${SEAPORTAL_REPORT_FILE:-}" ]]; then
  ts=$(date -u +%Y%m%d_%H%M%S)
  mkdir -p "$REPO_ROOT/tests/bench/reports"
  export SEAPORTAL_REPORT_FILE="$REPO_ROOT/tests/bench/reports/selftest_${ts}.jsonl"
fi

touch "$SEAPORTAL_REPORT_FILE"

# Detect a subagent runtime. The seaportal-opt skill spawns blind
# subagents via the Claude Code CLI (binary name `claude`). If we can't
# find any way to drive one, print a clear message and exit 0 — callers
# (CI, `seabench selftest --input`, tests) can then replay synthetic
# JSONL instead of failing the build.
RUNTIME=""
if [[ -n "${CLAUDE_CODE_AGENT_RUNTIME:-}" ]]; then
  RUNTIME="$CLAUDE_CODE_AGENT_RUNTIME"
elif command -v claude >/dev/null 2>&1; then
  RUNTIME="claude"
fi

if [[ -z "$RUNTIME" ]]; then
  cat >&2 <<EOF
selftest: no subagent runtime detected on this machine.

The selftest harness needs to spawn an AI subagent (via the Claude Code
CLI or an equivalent) to actually drive seaportal against the 10
curated tasks in:
  $GROUP_FILE

Nothing was recorded. To compute scoreboard metrics anyway — e.g. from
a previous run captured on a developer laptop, or from a synthetic
fixture in CI — re-invoke:

  seabench selftest --input <path-to-existing.jsonl>

Empty report file left at:
  $SEAPORTAL_REPORT_FILE

Exiting 0 so this is not treated as a build failure.
EOF
  exit 0
fi

# A subagent runtime was found. We don't pin a specific orchestration
# command here (the seaportal-opt skill owns that), we just hand off
# with the right env so `record.sh` writes to the file `seabench` will
# parse. The skill is expected to read group-selftest.md and the
# seaportal skill, then run each task in sequence.
cat >&2 <<EOF
selftest: subagent runtime detected ($RUNTIME).

This harness does not embed an opinionated spawn command — invoking
the seaportal-opt skill from outside an interactive Claude Code
session is intentionally not automated. Use one of:

  1. Run \`/seaportal-opt\` inside Claude Code, pointing it at:
       $GROUP_FILE
     with SEAPORTAL_REPORT_FILE=$SEAPORTAL_REPORT_FILE
  2. Replay a prior JSONL:
       seabench selftest --input <path>

Empty report file left at:
  $SEAPORTAL_REPORT_FILE
EOF
exit 0
