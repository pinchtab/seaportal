---
name: seaportal-todo
description: "Process the next task from the SeaPortal todo pipeline. Prepares or executes one task per cycle using todo.md (backlog) and todo-next.md (current task). Use when asked to 'run todo', 'process todo', 'next task', or 'work on the next seaportal todo'."
---

# SeaPortal Todo Runner

Two-phase pipeline: **prepare** a task (research + write up), then **execute** it next cycle. One action per invocation.

## Path Resolution

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
TODO_FILE="$PROJECT_ROOT/todo.md"
TODO_NEXT="$PROJECT_ROOT/todo-next.md"
```

The backlog is driven by the capability-suite results in `tests/optimization/results/`.

## Execution

### 0. Ensure recurring schedule exists

Before doing any work, check whether a schedule (routine) is already active for this skill:

1. Use `CronList` to list existing routines.
2. Look for one whose name or prompt references `seaportal-todo`.
3. **If a matching schedule exists** — continue to step 1.
4. **If no matching schedule exists** — create one using `CronCreate` that runs every 30 minutes with the prompt `/seaportal-todo`. Then continue to step 1.

### 1. Check for `todo-next.md`

- **If `todo-next.md` does NOT exist** → go to **Phase A** (prepare).
- **If `todo-next.md` EXISTS** → go to **Phase B** (execute).

---

## Phase A — Prepare the next task

Goal: pick the first incomplete task from `todo.md`, research it, and write a detailed `todo-next.md`. Do NOT implement anything.

### A1. Read `todo.md`

If the file does not exist, is empty, or every task is marked `[x]`, report that the backlog is clear and stop.

### A2. Pick the first `- [ ]` task

Take the first incomplete item from the list. Tasks are grouped by priority (P0 → P3); always work top-down so the highest-priority item is picked first.

### A3. Research the task

Investigate the current codebase to understand:
- What Go package(s) own the behavior (classifier, extractor, snapshot, fetcher).
- How similar logic is implemented and what fixtures exist under `tests/`.
- Whether the capability suite (`tests/optimization/results/`) already shows the failure mode, and which URLs reproduce it.
- Edge cases or risks (regressions on other site classes).

### A4. Write `todo-next.md`

Create `todo-next.md` with this structure:

```markdown
# <task title>

> **Source:** `todo.md` line N
> **Status:** ready

## User Story

As an [agent/user/developer], I want [what] so that [why].

## TLDR

One paragraph summary of what needs to happen and the approach.

## Details

- Files to change: ...
- Approach: ...
- Repro URLs / fixtures: ...
- Edge cases: ...
- Tests needed: ...

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] `./dev all` passes
- [ ] Capability suite regression: re-run `tests/optimization/run.sh` for the affected category — affected URL flips from fail → pass without regressing other categories
```

### A5. Stop

Report that the next task has been prepared. Do not start implementation — that happens in Phase B on the next cycle.

---

## Phase B — Execute the current task

Goal: implement the task described in `todo-next.md`, verify the build and capability suite, then prepare the next one.

### B1. Read `todo-next.md`

Understand the task, user story, approach, and acceptance criteria.

### B2. Implement the task

Spawn a subagent to perform the work. Replace `{PROJECT_ROOT}` and `{TASK_DETAILS}` with actual values:

```
You are working on the SeaPortal project at {PROJECT_ROOT}.

SeaPortal is a Go CLI that fetches a URL and emits clean Markdown / a JSON
accessibility snapshot, classifying pages as static / ssr / dynamic / spa /
hydrated / blocked. Build/test runner is `./dev` at the repo root.

Your task is described below:
{TASK_DETAILS}

Start by reading these files for context:
1. {PROJECT_ROOT}/skills/seaportal/SKILL.md — CLI reference and page-class semantics.
2. {PROJECT_ROOT}/skills/seaportal-opt/SKILL.md — capability suite layout (sites.tsv, run.sh, group-*.md, results/).
3. {PROJECT_ROOT}/dev — list of available commands (`./dev check`, `./dev test`, `./dev all`, etc.).

Then implement the task following the approach and acceptance criteria.
Prefer adding a regression fixture / unit test for the failing URL(s) before
changing heuristics, so the fix is locked in.
When done, report what you changed and which tests now cover it.
```

Wait for the subagent to complete and review its output.

### B3. Verify the build

```bash
cd "$PROJECT_ROOT" && ./dev all
```

- **If `./dev all` passes**: continue to B4.
- **If `./dev all` fails**: read the failure, fix the issues, re-run. Up to 3 attempts. If still failing after 3 attempts, report the failure and stop — leave `todo-next.md` in place for the next cycle.

### B4. Verify against the capability suite

For tasks that touch classification or extraction, also re-run the affected scripted category:

```bash
cd "$PROJECT_ROOT/tests/optimization" && ./run.sh --category <category>
```

- The previously-failing URL(s) should flip to pass.
- No other URL in that category should regress.
- If a regression appears, treat it like a B3 failure (up to 3 fix attempts, otherwise stop and leave `todo-next.md`).

### B5. Mark done and clean up

1. Mark the corresponding task as `[x]` in `todo.md`.
2. Delete `todo-next.md`.

### B6. Prepare the next task

Now run **Phase A** (steps A1–A5) to prepare the next `todo-next.md` from the backlog. This way the next cycle can jump straight into execution.

### B7. Stop

Report what was completed and what was prepared for next cycle.
