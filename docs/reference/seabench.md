# seabench

`seabench` is SeaPortal's benchmark / evaluation harness, built from `cmd/seabench`:

```bash
go build -o seabench ./cmd/seabench
./seabench <subcommand> [flags]
```

Most lanes run in-process — no subprocesses, no LLM. The offline lanes (`eval`/`classify`/`tokens`/`diff`/`stress`/`cachebench`) score against `testdata` fixtures using `must_include` / `must_exclude` substring lists in the corpus YAML. The `sweep` lane is the exception: it makes **live network requests** against a list of real sites.

## Subcommands

| Command | Purpose |
|---------|---------|
| `eval` | Run the corpus through four in-process extractors (seaportal, go-readability, html-to-markdown, strip-tags baseline) and emit a Markdown report with per-extractor precision / recall / F1 plus baseline-relative time ratios |
| `stress` | Stress-run extraction against a fixture preset |
| `classify` | Run the classification corpus and report page-class accuracy |
| `tokens` | Report token counts per corpus entry |
| `cachebench` | Benchmark the on-disk cache under a hot/cold access mix |
| `diff` | Diff extractor outputs across the corpus |
| `selftest` | Replay a recorded run group and check for regressions |
| `sweep` | **Live**: fetch every site in a list, report a capability + latency sweep (pageClass / outcome / browser-routing decision / confidence / block rate / latency percentiles) plus classification accuracy when the list carries `expect_class` labels |
| `help` | Show usage |

## Usage

```bash
seabench eval [--corpus PATH] [--report-dir DIR] [--baseline]
seabench stress [--preset quick|small|medium|large] [--baseline FILE] [--output DIR] [--fixture PATH]
seabench classify [--corpus FILE] [--output DIR]
seabench tokens [--corpus FILE] [--output DIR]
seabench cachebench [--n 200] [--hot-ratio 0.8] [--hot N] [--cold N] [--seed 42] [--output DIR]
seabench diff [--corpus FILE] [--output DIR] [--snippet-chars 400]
seabench selftest [--input FILE.jsonl] [--group FILE.md] [--output DIR]
seabench sweep [--sites FILE] [--concurrency 16] [--timeout 15s] [--limit N] [--fast] [--scheme https] [--output DIR]
```

The default `eval` corpus lives at `tests/eval/corpus.yaml`.

Each lane also prints a one-line headline to stdout (e.g. `eval: seaportal
F1=0.763 …`, `classify: 39/40 correct (accuracy=0.975) …`), so you can read the
result without opening the report.

## `./dev bench all` — run everything, get a summary

```bash
./dev bench all
```

Runs every **offline** (deterministic, no-network) lane — `eval`, `classify`,
`tokens`, `cachebench`, `stress --preset quick` — and collects each lane's
headline into a single recap:

```
━━━ Bench summary ━━━
  eval: seaportal F1=0.763 (P=0.644 R=0.935). See tests/bench/reports/eval_…md
  classify: 39/40 correct (accuracy=0.975). See …
  tokens: all-mode mean ratio=0.481 (40 fixtures × 4 modes). See …
  cachebench: ttl-24h hit=84% p50=1ms, swr-10m hit=84%. See …
  stress: 202 urls/s, 100% success, p50=4ms (n=50, quick). See …
```

The live `sweep` lane is excluded (it hits the network and isn't reproducible);
run it explicitly when you want a real-world capability check.

## `sweep` (live)

```bash
seabench sweep                                   # default: competitors/top-1000-sites-tranco.csv
seabench sweep --sites tests/optimization/sites.tsv   # labelled TSV → also reports accuracy
seabench sweep --sites tests/optimization/holdout.tsv # held-out classifier validation (see below)
seabench sweep --limit 50 --concurrency 8        # quick sample
```

`tests/optimization/holdout.tsv` is a held-out labelled set (sites in neither the
eval corpus nor `sites.tsv`) used to measure classifier *generalization* — see
[classifier-validation.md](classifier-validation.md). The in-corpus `classify`
score (40/40) is fit, not generalization.

The site list is auto-detected per line:

- `rank,domain` CSV (Tranco) — header skipped, unlabelled
- `category<TAB>url<TAB>expect_class<TAB>expect_marker` TSV (`sites.tsv`) — labelled
- one bare domain or URL per line (bare domains get the `--scheme` prefix)

Output: `tests/bench/reports/sweep_<ts>.{json,md}`. The JSON carries one row per site; the Markdown has reliability counts, latency percentiles, pageClass / outcome / decision distributions, slowest sites, an error sample, and — when the list is labelled — a per-class accuracy table. Concurrency, `--timeout`, and a single retry bound the run.
