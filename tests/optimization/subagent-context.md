# Subagent Context — SeaPortal Capability Suite

You are running a capability test against the `seaportal` CLI. Your job is to solve open-ended navigation tasks using **only** the `seaportal` command (no curl, no browser, no other tools). The point is to find out how far an AI agent can get with seaportal alone.

## Tool reference

Before you start, read `skills/seaportal/SKILL.md` for the full CLI surface. Quick refresher:

```bash
seaportal <url>                              # Markdown + frontmatter (also writes ./renders/seaportal/…)
seaportal --json <url>                       # full Result struct
seaportal --snapshot <url>                   # a11y tree as JSON
seaportal --snapshot --format=compact <url>  # a11y tree as compact text (best for tokens)
seaportal --snapshot --filter=interactive --format=compact <url>   # only links/buttons/inputs
seaportal --fast <url>                       # bail early if browser needed
```

There is no session. Every navigation is a fresh HTTP GET.

## Workflow

1. Pick the right output format for the task (Markdown for reading; compact snapshot for finding the next link; JSON when you need classification fields).
2. From the result, decide:
   - **Did the page extract?** Read `pageClass`, `validationOk`, `needsBrowser`, `confidence`.
   - **If `spa` / `dynamic` / `blocked` or `needsBrowser: true`** → record an "escalate" outcome. Do not keep trying — this is a legitimate seaportal limit.
3. To follow a link: take its `href` from the Markdown or the snapshot, resolve it against the current URL if relative, and re-run `seaportal` on it. Track visited URLs to avoid loops.
4. Stop when the task's verification condition is met, or after a reasonable bound (≤ 6 hops per task).

## Recording

For each step:

```bash
./record.sh "<group>.<step>" "<outcome>" "<one-line note>"
# outcome ∈ pass | fail | escalate | escalate-paywall
```

(`record.sh` is a small wrapper that appends a JSON line to `results/agent_<timestamp>.jsonl`.)

`escalate` is a first-class outcome — it means seaportal correctly identified that a browser is needed. That is a feature, not a failure.

Use **`escalate-paywall`** instead of plain `escalate` when seaportal extracted a usable preview (abstract, headline, byline) but the substantive body is gated behind a subscription or partial-content login wall. Canonical example: Repubblica articles return an abstract while the full body is paywalled. The distinction matters because paywall escalations are a content-availability limit, not a tool capability limit — a real browser would not help.

## Rules

- Use only `seaportal`. No `curl`, no `wget`, no `Read`-ing external URLs, no Python, no other CLI fetchers.
- Token-efficiency matters: prefer `--snapshot --format=compact --filter=interactive` over full Markdown when you only need a link.
- Don't loop: bound each task at 6 invocations.
- Read group files in order, do every step in each assigned group.

## What we measure

- **Pass rate** per group.
- **Escalation rate** — % of steps where seaportal correctly said "needs browser".
- **Ops per step** — how many `seaportal` invocations the agent used.
- **Wrong-extraction rate** — pages where seaportal returned content but the agent's answer was wrong (the most important failure mode).
