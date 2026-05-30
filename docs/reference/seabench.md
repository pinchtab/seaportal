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

## `sweep` (live)

```bash
seabench sweep                                   # default: competitors/top-1000-sites-tranco.csv
seabench sweep --sites tests/optimization/sites.tsv   # labelled TSV → also reports accuracy
seabench sweep --limit 50 --concurrency 8        # quick sample
```

The site list is auto-detected per line:

- `rank,domain` CSV (Tranco) — header skipped, unlabelled
- `category<TAB>url<TAB>expect_class<TAB>expect_marker` TSV (`sites.tsv`) — labelled
- one bare domain or URL per line (bare domains get the `--scheme` prefix)

Output: `tests/bench/reports/sweep_<ts>.{json,md}`. The JSON carries one row per site; the Markdown has reliability counts, latency percentiles, pageClass / outcome / decision distributions, slowest sites, an error sample, and — when the list is labelled — a per-class accuracy table. Concurrency, `--timeout`, and a single retry bound the run.
