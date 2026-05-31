# SeaPortal Capability Suite

Reach out to a broad set of public websites and verify what seaportal can and cannot do. Two complementary lanes:

1. **Scripted lane** — `run.sh` iterates `sites.tsv`, runs `seaportal --json --fast <url>`, asserts the page classification and an expected content marker. Produces a JSON report + Markdown summary in `results/`. Fast, reproducible, no AI needed.
2. **Agent lane** — `subagent-context.md` + the `group-*.md` files drive an AI agent to solve open-ended navigation tasks using only the `seaportal` CLI (per the `seaportal` skill). Measures whether an agent can actually drive the tool, not just whether the binary works.

## Files

| Path | Purpose |
|---|---|
| `sites.tsv` | Master list: `category \t url \t expect_class \t expect_marker` |
| `run.sh` | Scripted runner |
| `subagent-context.md` | Instructions handed to each agent in the agent lane |
| `group-00.md` … `group-13.md` | Task groups for the agent lane |
| `results/` | Run outputs (`run_<ts>.json`, `run_<ts>.md`) |

## Site categories in `sites.tsv`

- `static` — minimal HTML reference pages
- `wikipedia` — multilingual SSR articles
- `news` — text-mode and full news properties
- `docs` — technical documentation
- `blog` — long-form personal/engineering blogs
- `qa` — Q&A and forum SSR pages
- `code` — code hosting (raw + rendered)
- `saas` — marketing landing pages
- `gov` — government sites
- `academic` — papers and journals
- `feeds` — RSS / XML feed pages
- `spa` — JS-only sites; expected escalation signal
- `ecom` — e-commerce, often hydrated or blocked
- `search` — search engines
- `finance` — markets, regulators, paywalled finance
- `i18n` — non-Latin / RTL pages
- `edge` — RFCs, httpbin, W3C, edge cases

Goal: cover every `pageClass` seaportal emits (`static`, `ssr`, `hydrated`, `spa`, `dynamic`, `blocked`) at least twice, and surface real-world signal-to-noise on the classifier.

## Running

```bash
cd tests/optimization
./run.sh                          # full suite, 4 parallel
./run.sh --category wikipedia     # one category
./run.sh --jobs 8 --timeout 45    # tuned
```

Exit code is non-zero if any site failed its assertion.

## Adding sites

Append a line to `sites.tsv`:
```
category<TAB>url<TAB>expect_class<TAB>expect_marker
```
- `expect_class`: `static|ssr|hydrated|spa|dynamic|blocked|any`. Use `any` while you're learning what seaportal labels a site.
- `expect_marker`: a substring expected somewhere in the extracted Markdown, or `-` to skip. Pick something stable (a brand name, a heading, a domain term).

## Reference numbers

Initial site count: ~50, across 15 categories. Expand by adding rows — no code changes needed.
