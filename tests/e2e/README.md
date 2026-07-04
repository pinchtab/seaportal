# SeaPortal CLI E2E tests

Docker-based end-to-end tests that exercise the built `seaportal` binary against
nginx-served fixtures. Scenarios live in `scenarios/NN-*.sh` and are auto-sourced
by `run-all.sh` in filename order; each reports into a pass/fail tally and
`results/summary.json`.

## Running

```bash
# Docker (recommended — runs the full suite incl. scrape scenarios)
./run-local.sh

# Without Docker (extraction scenarios only; scrape scenarios self-skip)
./run-local.sh --no-docker
```

`run-local.sh` builds the CLI first, then either brings up docker-compose
(`fixtures` + `scrapesite` + `crawlsite` + `runner`) or serves `./fixtures`
locally and runs the scenarios directly.

## Services / fixtures

| Service | Docroot | Serves |
|---------|---------|--------|
| `fixtures` | `./fixtures` | flat extraction fixtures (scenarios 01–05) |
| `scrapesite` | `./fixtures/site` | multi-page scrape site with sitemap-index + robots (ALP-014) |
| `crawlsite` | `./fixtures/site-nosm` | no-sitemap site for the crawl-fallback path |

The runner receives `FIXTURES_URL`, `SCRAPE_SITE_URL`, and `CRAWL_SITE_URL`;
`common.sh` provides defaults and the `require_host` guard.

## Scrape scenarios (06–11)

| Scenario | Covers |
|----------|--------|
| `06-scrape-basic` | sitemap discovery, pattern groups, populated pages |
| `07-scrape-sampling` | `--max-pages` / `--max-per-pattern` caps, strategies, `--full`, determinism |
| `08-scrape-robots-filters` | robots respect toggle + include/exclude patterns |
| `09-scrape-crawl-fallback` | no-sitemap → bounded homepage crawl (same-host, budget) |
| `10-scrape-output` | `--output json` / `md` / `directory` renderers |
| `11-scrape-horizontal` | large sitemap stays bounded + fast ("doesn't explode") |

The scrape scenarios need the dedicated host-root fixtures (`scrapesite` /
`crawlsite`) — their robots.txt and sitemap.xml must resolve at a host root and
the sitemap `<loc>` hosts must match the scraped base URL. Under `--no-docker`
those hosts aren't up, so each scrape scenario calls `require_host` and **skips
cleanly** (the extraction suite still runs).
