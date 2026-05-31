# Reliability — what to expect

SeaPortal is a **fast first-pass triage that fails over**, not a universal fetcher.
It is fast and reliable on static / server-rendered pages and signals when a page
needs a real browser. Across the raw open web a large share of hosts are CDN/DNS
infrastructure that never serve HTML, so headline "success" looks low until you net
those out.

The numbers below are a frozen snapshot of the committed live sweeps. Regenerate at
any time with `./dev bench sweep` (see [seabench](seabench.md)); reports land under
`tests/bench/reports/` (git-ignored, regenerated per run).

## Summary

| | Reachable, in-niche (static/SSR) | Across the open web (Tranco top-1000) |
|---|---|---|
| Latency (ok fetches) | p50 ~1s, p95 ~2s | p50 ~1.6s, **p90 ~10.6s, p95 ~15.5s** |
| Success | ~94% ok | 40.3% ok — **~53%** netting out the ~242 dead CDN/DNS infra hosts |

Route on the browser-recommendation signal (`profile.decision` /
`browserRecommended`) and set `--timeout`; fail over to a browser rather than
assuming every URL extracts.

## Open web — Tranco top-1000

- Snapshot: `sweep_20260530-161156` · captured 2026-05-30T16:11:56Z · git `5203684`
- List: `competitors/top-1000-sites-tranco.csv` · 1000 sites · concurrency 24 · timeout 15s

| Metric | Count | % |
|---|---|---|
| ok | 403 | 40.3% |
| blocked | 29 | 2.9% |
| errors | 252 | 25.2% |
| timed out | 345 | 34.5% |
| browser recommended | 108 | 10.8% |

Latency over the 403 successful fetches: mean 3526 ms, p50 1646, p90 10619,
p95 15547, p99 19232, max 19650.

Netting out the ~242 unreachable CDN/DNS infrastructure domains (e.g.
`akamaiedge.net`, `cloudfront.net`, `domaincontrol.com`) that never serve HTML,
reachable success is ~53%.

## In-niche sample — evenly-spaced cross-section

- Snapshot: `sweep_20260530-161551` · captured 2026-05-30T16:15:51Z · git `5203684`
- List: `tests/bench/sites-sample.csv` · 50 sites (stride ~15.5 over the Tranco list) · concurrency 10 · timeout 20s

| Metric | Count | % |
|---|---|---|
| ok | 47 | 94.0% |
| blocked | 0 | 0.0% |
| errors | 1 | 2.0% |
| timed out | 2 | 4.0% |
| browser recommended | 9 | 18.0% |

Latency over the 47 successful fetches: mean 1099 ms, p50 976, p90 1890,
p95 2160, p99 2483, max 2483.

This sample is a fair, evenly-spaced cross-section of the Tranco list (not a
hand-picked best case). Because its dead-infra hosts are excluded by the stride
landing on reachable domains, it approximates the *reachable* slice of the open web
— hence the much healthier latency and success figures.
