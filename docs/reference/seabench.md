# seabench

`seabench` is SeaPortal's benchmark / evaluation harness, built from `cmd/seabench`:

```bash
go build -o seabench ./cmd/seabench
./seabench <subcommand> [flags]
```

All extractors run in-process — no subprocesses, no network, no LLM. Scoring comes from `must_include` / `must_exclude` substring lists declared in the corpus YAML.

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
```

The default `eval` corpus lives at `tests/eval/corpus.yaml`.
