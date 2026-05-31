---
name: seaportal-opt
description: "Run the SeaPortal capability suite. Two lanes: (1) a scripted runner that hits ~50 real public websites and asserts pageClass + content markers, and (2) an agent lane that spawns blind subagents to solve open-ended navigation tasks using only the seaportal CLI. Use when asked to 'run seaportal capability tests', 'benchmark seaportal', 'check seaportal against real sites', or 'verify seaportal across the web'."
---

# SeaPortal Capability Suite

Reach out to as many different public websites as possible and verify what seaportal can and cannot do. The suite has two complementary lanes — run both for a full picture, or just the scripted lane for a quick CI-style smoke.

## Path resolution

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
OPT_DIR="$PROJECT_ROOT/tests/optimization"
```

## Prerequisites

```bash
command -v seaportal >/dev/null || echo "seaportal CLI missing"
command -v jq        >/dev/null || echo "jq missing"
```

If `seaportal` is missing, install with `npm i -g seaportal` or `go install github.com/pinchtab/seaportal/cmd/seaportal@latest`.

## Lane 1 — Scripted runner (fast, no AI)

```bash
cd "$OPT_DIR"
./run.sh                          # full suite, 4 parallel, 30s timeout per site
./run.sh --category wikipedia     # one category at a time
./run.sh --jobs 8 --timeout 45    # tune
```

What it does: reads `sites.tsv`, runs `seaportal --json --fast` against each URL, asserts the `pageClass` matches (or is `any`) and the expected content marker appears in the extracted Markdown.

Output: `results/run_<ts>.json` (machine) and `results/run_<ts>.md` (human summary with pass/fail per category and per-class distribution).

Adding sites: append a row to `sites.tsv`:
```
category<TAB>url<TAB>expect_class<TAB>expect_marker
```
Use `expect_class=any` when you don't yet know what seaportal will label the site.

## Lane 2 — Agent lane (capability of an AI driving seaportal)

The agent lane measures whether an AI agent, given only the `seaportal` skill, can complete realistic navigation tasks. Tasks are in `group-00.md` through `group-09.md`.

### 0. Create per-agent report files

```bash
RESULTS_DIR="$OPT_DIR/results"
TIMESTAMP=$(date -u +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

for agent in A B C; do
  : > "$RESULTS_DIR/agent${agent}_${TIMESTAMP}.jsonl"
done
```

### 1. Spawn parallel subagents

Use the **Agent** tool with `run_in_background: true`. Split the 10 groups (groups 0–9) across three agents:

- **Batch A**: groups 0–3
- **Batch B**: groups 4–6
- **Batch C**: groups 7–9

Same prompt template for each, replacing `{START}`, `{END}`, `{START_PAD}`, `{END_PAD}`, `{PROJECT_ROOT}`, and `{REPORT_FILE}`:

```
You are running the SeaPortal capability suite. Your job is to execute groups {START} through {END}.

CRITICAL: Use ONLY the `seaportal` CLI. Do not curl, wget, fetch URLs with Read, or use any other network tool.

Your report file is: {REPORT_FILE}
Export SEAPORTAL_REPORT_FILE={REPORT_FILE} before any record.sh call.

Start by reading these files:
1. {PROJECT_ROOT}/tests/optimization/subagent-context.md — workflow, recording rules, what we measure.
2. {PROJECT_ROOT}/skills/seaportal/SKILL.md — full CLI reference.
3. {PROJECT_ROOT}/tests/optimization/group-{START_PAD}.md through group-{END_PAD}.md — your tasks.

DO NOT read sites.tsv (that's the scripted lane's answer key) or any file under results/.

For each step:
- Pick the right seaportal mode (Markdown / JSON / compact snapshot / filtered snapshot).
- Decide pass / fail / escalate / escalate-paywall based on the task's verify clause and seaportal's classification. Use `escalate-paywall` when an abstract/preview was extracted but the body is gated behind a subscription; use plain `escalate` for SPA / bot-block / captcha.
- Record with: {PROJECT_ROOT}/tests/optimization/record.sh "<group>.<step>" "<outcome>" "<note>"

Bound: ≤ 6 seaportal invocations per step. Track visited URLs to avoid loops.
```

### 2. Monitor progress

```bash
wc -l "$RESULTS_DIR"/agent*_${TIMESTAMP}.jsonl
```

Expected line counts: A ~17 steps, B ~13 steps, C ~13 steps ≈ 43 total.

### 3. Summarize

```bash
jq -s '{
  total: length,
  pass:              [.[]|select(.outcome=="pass")]              | length,
  fail:              [.[]|select(.outcome=="fail")]              | length,
  escalate:          [.[]|select(.outcome=="escalate")]          | length,
  escalate_paywall:  [.[]|select(.outcome=="escalate-paywall")]  | length
}' "$RESULTS_DIR"/agent*_${TIMESTAMP}.jsonl
```

Show the user the pass / fail / escalate / escalate-paywall breakdown and any `fail` rows with their notes. When summarising for the user, add `escalate + escalate-paywall` to get the total "seaportal handed off cleanly" rate, and call out `escalate-paywall` separately since it reflects content availability rather than tool capability.

## What we measure

| Metric | Lane | Meaning |
|---|---|---|
| Scripted pass rate | 1 | Sanity: does the binary classify and extract real sites correctly? |
| Class distribution | 1 | How does seaportal label the wild web? |
| Agent pass rate | 2 | Can an AI drive seaportal? |
| Agent escalation rate | 2 | % of steps where seaportal correctly said "needs browser" — feature, not failure. |
| Paywall rate | 2 | % of steps where seaportal got a preview but the body was paywalled — content limit, not a tool limit. |
| Ops/step | 2 | Token-efficiency proxy. |

## Eval bake-off

Quality scorer comparing seaportal against `go-readability`, `html-to-markdown`, and a `strip-tags` baseline on `tests/eval/corpus.yaml`. All four extractors run in-process — no network, no LLM.

```bash
go run ./cmd/seabench eval                # writes tests/bench/reports/eval_<ts>.md
go run ./cmd/seabench eval --baseline     # also overwrites eval_baseline.md
go run ./cmd/seabench eval --corpus path/to/corpus.yaml --report-dir out/
```

Read the headline table for micro-averaged Precision / Recall / F1 and machine-independent time ratios (vs strip-tags). Per-fixture detail tables follow; `notes` column flags `no-signal` (skipped from math) or `skipped(<50b)` (extractor produced nothing).

## File locations (relative to project root)

| Path | Purpose |
|---|---|
| `tests/optimization/sites.tsv` | Scripted-lane site list |
| `tests/optimization/run.sh` | Scripted runner |
| `tests/optimization/record.sh` | Agent-lane per-step recorder |
| `tests/optimization/subagent-context.md` | Agent instructions |
| `tests/optimization/index.md` | Suite overview |
| `tests/optimization/group-00.md` … `group-09.md` | Agent-lane tasks |
| `tests/optimization/results/` | Outputs (gitignored if desired) |
| `skills/seaportal/SKILL.md` | CLI reference loaded by subagents |
