---
name: seaportal-dev
description: Develop and contribute to the SeaPortal project. Use when working on SeaPortal source code, adding features, fixing bugs, running tests, or preparing PRs. Triggers on "work on seaportal", "seaportal development", "contribute to seaportal", "fix seaportal bug", "add seaportal feature".
---

# SeaPortal Development

SeaPortal is a Go CLI that fetches a URL and emits clean Markdown / a JSON
accessibility snapshot, classifying pages as static / ssr / dynamic / spa /
hydrated / blocked. HTTP-only — for JS-heavy pages, escalate to pinchtab.

## Project Location

```bash
cd ~/dev/prj/pinchtab/seaportal
```

## Dev Commands

All development commands run via `./dev`:

| Command | Description |
|---|---|
| `./dev build` | Build the application |
| `./dev run` | Build & run with test URL |
| `./dev check` | All checks (format, vet, lint, test) |
| `./dev test` | Run unit tests |
| `./dev test race` | Run tests with race detector |
| `./dev coverage` | Run tests with coverage |
| `./dev lint` | Run golangci-lint |
| `./dev fmt` | Format code |
| `./dev vet` | Run go vet |
| `./dev fuzz` | Run fuzz tests (30s) |
| `./dev bench [sub]` | Build + run seabench (`./dev bench help` for subcommands) |
| `./dev opt baseline` | Run the scripted optimization lane |
| `./dev e2e` | E2E tests (Docker) |
| `./dev smoke` | Smoke tests (slow fixture suite) |
| `./dev all` | check + test + e2e (pre-push gate) |
| `./dev doctor` | Check dev environment |
| `./dev install skills` | Symlink `./skills` into `./.claude/skills` |

## Architecture

```
cmd/
  seaportal/        # CLI entry: extract (default), sitemap, feed, mcp
  seabench/         # Benchmark + competitor-comparison harness
                    # subcommands: eval, stress, classify, tokens,
                    # cachebench, diff, selftest, sweep (live)
internal/
  engine/           # Core extraction pipeline + cache + classifier
    mock/           # Record/replay HTTP mock for hermetic tests
    leakcheck/      # Goroutine-leak guard for tests
  mcp/              # MCP (Model Context Protocol) server (over stdio)
  testserver/       # Hermetic HTTP fixture server
    fixture/        # Declarative-route helper for per-test servers
  quality/          # Content quality scoring
testdata/           # Test fixtures, organised by class:
  static/           # Pre-rendered static HTML (8 fixtures)
  ssr/              # Server-rendered (10 fixtures)
  hydrated/         # SPA with full first-paint (2 fixtures)
  dynamic/          # JS-rendered, low first-paint (5 fixtures)
  blocked/          # Bot challenges, captcha, auth walls (11+ fixtures)
  multilingual/     # de, es, zh, ar, ru, fr, ja Wikipedia pages
  adversarial/      # Robustness inputs (10MB, deep nesting, billion-laughs)
  regression/       # Bug-specific minimal repros
  snapshot/         # golden/ holds frozen a11y-snapshot outputs
  feeds/            # RSS/Atom/JSON Feed (incl. malformed-CDATA repro)
  sitemaps/         # sitemap.xml (incl. malformed-truncated repro)
  schema/           # Schema YAML example for ApplySchema
  index/            # Index-page fixtures
  fixtures/         # Misc legacy
  mocks/            # Replay JSONL for internal/engine/mock
tests/
  bench/            # seabench reports (eval_*, sweep_*, classify_*, ...) +
                    # per-phase pprof baselines
    sites-sample.csv  # 50-site live-sweep smoke list (evenly spaced by rank)
  e2e/              # Docker-based CLI scenarios (5 scripts under scenarios/)
  eval/             # Quality eval corpus.yaml (40 entries, all classes)
  optimization/     # Agent-driven capability suite
                    # group-00..13.md + group-selftest.md (curated 10)
                    # sites.tsv (143 real-site assertions, 18+ categories)
  release/          # .goreleaser.yml sanity test (matrix + naming)
competitors/        # Competitor repos for comparison +
                    # top-1000-sites-tranco.csv (live-sweep default list,
                    # DNS-dead infra domains pruned)
skills/             # Project-local Claude Code skills
  seaportal/        # Read-only web fetching skill (consumer-facing)
  seaportal-dev/    # This skill — dev workflow reference
  seaportal-opt/    # Capability-suite runner
```

## Pre-push gate

`./dev all` is the canonical pre-push gate:

1. `check` — gofmt, go vet, go build, golangci-lint
2. `test` — full unit suite (with `-count=1`)
3. `e2e` — Docker-based scenarios in `tests/e2e/`

Anything that touches `internal/engine/extract.go` should also run
`./dev bench eval` to confirm no regression on the corpus.

## Evaluating a change

Pick the lane by what the change touches. The first four are offline,
deterministic, and CI-safe (fixtures only); `sweep` is the only **live**
(real-network) lane.

| Change touches | Run | Reads |
|---|---|---|
| Extraction / cleanup / readability | `./dev bench eval` | quality F1 vs 3 baselines |
| Classifier / SPA-detect / routing decision | `./dev bench classify` | confusion matrix vs corpus labels |
| Cache | `./dev bench cachebench` | hit-rate + latency |
| Link/token shaping | `./dev bench tokens` | token ratio per retention mode |
| Real-world reach, latency, block/escalation rates | `./dev bench sweep` | live capability + latency sweep |

`sweep` fetches a whole site list live and reports reliability counts
(ok / blocked / errors / timed-out / browser-recommended), latency
percentiles, and pageClass / outcome / decision distributions; when the list
carries `expect_class` labels it also reports classification accuracy.

```bash
./dev bench sweep --sites tests/bench/sites-sample.csv --timeout 20s   # fast 50-site smoke (~30s)
./dev bench sweep                                                      # full list (competitors/top-1000-sites-tranco.csv, 776 sites)
./dev bench sweep --sites tests/optimization/sites.tsv                 # labelled TSV → also reports accuracy
```

Flags: `--concurrency` (default 16), `--timeout` (per-site, default 15s;
bump to 20–30s to avoid counting slow-but-fine sites as timeouts), `--limit N`,
`--fast`. Reports land in `tests/bench/reports/sweep_<ts>.{md,json}`. The
list auto-detects three forms: `rank,domain` CSV (Tranco), `sites.tsv` TSV,
or one URL/domain per line. Regenerate the sample at any size with an
evenly-spaced stride over the cleaned list (`tests/bench/sites-sample.csv`).

## Adding a feature

1. Read `CONTRIBUTING.md` (regression-test-per-fix convention applies).
2. Add a regression test FIRST (see `// regression: <slug>` markers in the
   codebase for prior art).
3. Implement.
4. Re-run `./dev all`. Run `./dev bench eval` if extraction was touched.
5. Update `skills/seaportal/SKILL.md` if a user-visible CLI flag changed.

## Architectural constraints

These constraints come from the project vision. Do not violate without
discussion:

- **No site-specific code.** Cross-cutting infrastructure (Cloudflare, the
  Wikidata edit-pencil pattern) is acceptable; per-host code is not.
- **HTTP-only.** No JS execution, no browser fallback. Escalate to pinchtab.
- **No LLM calls.** SeaPortal is *input* to LLMs, not a wrapper around them.
- **Sub-2s extraction** is the target for typical (non-pathological) inputs.

## Common tasks

- **Adding a new corpus fixture**: drop the HTML under `testdata/<class>/`
  (static / ssr / hydrated / dynamic / blocked / multilingual / etc.), add
  a `corpus.yaml` entry with `must_include` markers in the document's
  native script, run `./dev bench eval` to verify F1.
- **Adding a real-site assertion**: append to `tests/optimization/sites.tsv`
  with the observed `pageClass` (run `./seaportal --json <url>` first; never
  ship aspirational expectations). Re-run `./dev opt baseline` to confirm
  ≥85% overall pass rate.
- **Adding a new seabench subcommand**: mirror `cmd/seabench/classify.go`
  shape (subcommand file + test + main.go dispatch + printUsage line).
- **Adding a new fuzz harness**: append to `internal/engine/fuzz_test.go`,
  seed with 4-6 inputs (1-2 from real `testdata/`).
- **Running the agent self-test**: `./dev opt selftest` runs the curated
  10-task subset and emits completion-rate / avg-ops / escalation-correctness
  metrics. Replay-friendly via `seabench selftest --input <jsonl>`.
- **Releasing**: see `RELEASE.md`.

## Available `./dev` aliases

Beyond the table above:
- `./dev bench eval` — extraction quality bake-off (seaportal vs 3 baselines)
- `./dev bench classify` — page-class confusion matrix vs corpus labels
- `./dev bench tokens` — token-efficiency ratio per LinkRetention mode
- `./dev bench stress --preset quick|small|medium|large` — sustained throughput
- `./dev bench cachebench` — cache hit-rate + latency under mixed traffic
- `./dev bench diff` — pairwise output diff across cleanup variants
- `./dev bench sweep` — **live** capability + latency sweep over a site list
  (default `competitors/top-1000-sites-tranco.csv`; `--sites tests/bench/sites-sample.csv` for a fast smoke)
- `./dev opt baseline` — scripted real-site capability suite (`sites.tsv`)
- `./dev opt selftest` — agent-driven 10-task curated suite
