# Code Quality & Architecture Audit — seaportal

- **Date:** 2026-07-11 (tree at commit `738dcdb`)
- **Scope:** all non-test production Go (~21k LOC), test suite architecture (~21k LOC), CI config, npm packaging. Read-only audit; no code changed.
- **Method:** five parallel review passes — architecture/layering, single-responsibility/big files, duplication, Go idioms/robustness, test infrastructure — cross-verified against source with file:line evidence.

---

## Executive summary

The codebase is in better shape than its file sizes suggest: layering is genuinely clean (root facade → `internal/engine`; `internal/mcp` is a self-contained JSON-RPC transport with zero project imports — MCP is **not** implemented twice), error wrapping is consistent, there are no panics in library code, and the test/CI infrastructure is above average (race lane, golden regen workflow, tag-gated heavy fixtures, gosec, golangci-lint).

The five problems that matter most:

1. **The scrape pipeline runs without any `SecurityPolicy`** — the MCP `scrape_site` tool crawls arbitrary URLs with no SSRF guard and unbounded body reads, while every sibling verb applies `DefaultSecurityPolicy()`. This is drift caused by parallel option structs (T01).
2. **`FromURLWithOptions` is a 737-line god function** with its result-finalization tail copy-pasted 4×, the post-content pipeline 4×, and quality-metric refresh 10× — the single biggest source of drift risk (T10–T12).
3. **`Result` (234 fields) and `Options` (48 fields, 21 bools) are god structs** that every one of the 66 engine files depends on; they are the coupling backbone that blocks any package split (T13–T14).
4. **The HTTP/2 uTLS path leaks a TLS connection + goroutine per successful HTTPS request** (T02).
5. **Two well-built quality gates never run:** `-tags=integration` (MCP conformance + latency budget) and `-tags=allocs` are wired into nothing, and CI ignores `testdata/**` changes entirely (T30–T32).

**Resolved during audit:** the in-flight ALP-048 rate-limiter rewrite initially left `scrape.go:301` calling the old signature (broken build); commit `738dcdb` landed complete — all 4 `limiter.Wait` call sites are ctx-aware with handled errors. Residual items are T04/T05.

## What's already in good shape (no action)

- Root facade `seaportal.go`/`scrape.go`: disciplined, fully doc-commented alias layer; zero copied logic.
- `internal/mcp/server.go`: clean protocol/registration split with `cmd/seaportal/mcp.go`; handler panics recovered.
- Sentinel errors + `%w` wrapping consistent; SSRF/redirect-revalidation design in `security.go` is solid (the problem is paths that don't opt in).
- `runFetchPool` (`scrape_pool.go:46-93`): textbook bounded worker pool.
- `tables.go`, `index_extract.go`, `preprocess.go`, `dedupe.go`: internally well decomposed.
- Test pyramid healthy: pure functions tested directly, heavy work tag-gated, goldens diffed with `firstDiffLines`, only 3 `reflect.DeepEqual` uses.
- Package-level state is almost all immutable compiled regexes; a single benign `init()` (`sanitize.go:51`).

---

## Task list

Severity: what it costs to leave it. Effort: S (≤½ day) / M (1–2 days) / L (multi-day). IDs are stable for filing as Alp tasks.

### P0 — Security & correctness

#### T01 — Thread `SecurityPolicy` through the scrape pipeline · **HIGH / M**
`ScrapeOptions` (`internal/engine/scrape.go:73-86`) has no `Security` field, so every scrape-path fetch runs unguarded: `scrape.go:307`, `scrape_discover.go:79,115,163` pass `FetchBytesOptions{Timeout, UserAgent}` only. With `Security == nil`, `limitedReadAll` (`security.go:325-327`) is an unlimited `io.ReadAll` and decompression caps are 0 = unbounded. Exposed through MCP `scrape_site` (`cmd/seaportal/mcp.go:230-313`) — whose sibling `fetch_url` explicitly applies `DefaultSecurityPolicy()` "because MCP is the most exposed entrypoint". Also: `fetchSitemap` gunzips `.gz` sitemaps with uncapped `io.ReadAll(gz)` (`sitemap.go:158-168`) — a decompression bomb even when a policy is passed.
**Task:** add `Security *SecurityPolicy` to `ScrapeOptions`, default it in `normalized()`, thread into every `FetchBytesOptions` in scrape/discover and the `Options` built in `scrape_pool.go:33`; apply `DefaultSecurityPolicy()` in MCP `scrape_site` and CLI scrape (with `--allow-internal` escape hatch like the other verbs); cap the sitemap gunzip via `limitedReadAll(gz, MaxDecompressedBytes, ErrDecompressTooLarge)`.

#### T02 — Fix HTTP/2 connection + goroutine leak in `chromeTransport` · **HIGH / M**
`utls.go:123-143, 269-278`: each HTTPS request dials a fresh TLS conn; on h2 a throwaway `http2.Transport.NewClientConn(tlsConn)` spawns a readLoop goroutine, and `tlsConn` is closed only on the error path. `resp.Body.Close()` closes the stream, not the connection — long-running processes (MCP server, 50-page scrapes) accumulate fds and goroutines. HTTP/1.1 path is safe (`DisableKeepAlives: true`).
**Task:** cache/reuse `ClientConn`s per host, or wrap `resp.Body` so Close also closes `tlsConn`/h2 conn after drain. Add a leak test using the existing `internal/engine/leakcheck` package.

#### T03 — Fix robots.txt fetching: lock-held network call, no ctx, wrong transport, 3× caches · **HIGH / M**
(a) `fetchAndCache` (`robots.go:113-144`) holds `c.mu.Lock()` across a 5s network call — all `IsAllowed`/`GetDelay` callers for *any* domain block while one domain's robots.txt is in flight. (b) The request uses `http.NewRequest` (no ctx) and a raw `http.Client{Timeout: 5s}` — bypassing uTLS, `Options.Proxy`, security policy, and the `Transport` test seam; a proxied deployment leaks direct robots.txt requests. (c) One scrape run builds **three** independent `CrawlDelayCache`s (`scrape_discover.go:41`, `scrape_discover.go:139`, `scrape.go:293`) — robots.txt re-fetched up to 3× per run. (d) The BFS discovery crawl (`crawlSameHost`, `scrape_discover.go:153-185`) honors Disallow but applies **no crawl-delay and no rate limiter**.
**Task:** fetch outside the lock (per-domain singleflight); use `http.NewRequestWithContext` and route through the shared fetch path; create one `CrawlDelayCache` + one `HostRateLimiter` in `ScrapeSite` and pass them down; apply `limiter.Wait` in `crawlSameHost`.

#### T04 — Replace error-string retry matching with typed checks · **MEDIUM / S**
`isRetryableError` (`http.go:125-139`) falls back to `strings.Contains(strings.ToLower(errStr), ...)` for `"connection reset"`, `"no such host"`, `"EOF"` — bare `"eof"` matches any error containing the substring, and `"no such host"` (NXDOMAIN, permanent) burns the full retry budget on every typo'd domain.
**Task:** use `errors.Is(err, io.EOF)`/`io.ErrUnexpectedEOF`, `net.DNSError.IsNotFound` (do **not** retry), delete the substring fallback.

#### T05 — Clamp robots.txt crawl-delay · **MEDIUM / S**
`parseRobotsTxt` accepts any positive delay (`robots.go:216-224`); a hostile `Crawl-delay: 86400` feeds straight into `limiter.Wait` (`scrape.go:301`) and stalls workers until the overall timeout — a guaranteed-useless scrape.
**Task:** clamp effective delay (e.g. `min(delay, 30s)` or a `MaxCrawlDelay` option); emit a warning when clamped.

#### T06 — Close remaining context-propagation gaps · **MEDIUM / S**
Despite ALP-043/048: `extract.go:400` calls `Security.ValidateURL(context.Background(), …)` while `reqCtx` exists 26 lines above (un-cancellable DNS in the SSRF gate); same in `extract_head_only.go:44`; the head-only GET (`extract_head_only.go:138`) and HEAD preflight (`extract.go:498`) aren't bound to the request ctx; `cmd/seaportal/main.go:549-559` hardcodes `context.Background()`; `internal/mcp/server.go:118-140` `serve` accepts ctx but the scanner loop never selects on `ctx.Done()`.
**Task:** thread the ctx through these five sites; add a `ctx.Err()` check per scan iteration in `mcp.serve`.

#### T07 — Delete (or converge on) the dead scrape fetch pool · **MEDIUM / S**
`fetchAll`/`fetchOne` (`scrape_pool.go:14-116`) are referenced only by their own tests; production uses `fetchAndAssemble` (`scrape.go:289`). Behaviour diverges materially: `fetchOne` uses the full `FromURLWithOptions` path (retry/cache/charset), production's `fetchAndAssemble` uses `FetchBytes` + `FromHTMLWithOptions` — bypassing retries, disk cache, redirect tracking, and header capture. Tests assert semantics of dead code.
**Task:** preferably converge `fetchAndAssemble` on `FromURLWithOptions` (also helps T01) and have `assemblePage` consume `Result`; otherwise delete `fetchAll`/`fetchOne` and port their order/cancellation tests to the shared `runFetchPool`.

#### T08 — Fix robots UA matching per RFC 9309 · **LOW / S**
`robots.go:315,342`: `strings.Contains(uaLower, agent) || strings.Contains(agent, "seaportal")` — a robots section named `bot` matches any UA containing "bot", and `User-agent: seaportal` sections bind even when impersonating Chrome (default UA is a Chrome string).
**Task:** longest token-prefix match on the configured UA's product token; decide explicitly whether self-identification overrides an impersonated UA.

#### T09 — Stop constructing `http.Transport` per request on proxy/security branches · **LOW / S**
`utls.go:87-102`: proxy and dial-guard branches allocate a new `http.Transport` inside `RoundTrip`, abandoning its idle pool without `CloseIdleConnections()`. fd-noisy under the scrape pool.
**Task:** build these transports once in the `chromeTransport` constructor (they depend only on immutable fields).

### P1 — Structural refactors (the big rocks)

Order matters: T10 → T11/T12 → T13/T14 → T15; T22 (package split) goes last.

#### T10 — Break up `FromURLWithOptions` (737 lines) into fetch stage + per-content-type handlers · **HIGH / L**
`extract.go:360-1096` inlines: data-URL shortcut, SSRF gate, per-domain timeout/UA/retry resolution, 4-layer client assembly, HEAD preflight, robots gate, crawl-delay, rate limit, cache lookup, retry fetch, 304 replay, decompress/charset, binary gate, a **full PDF pipeline** (704-797), a **full raw-JSON/XML pipeline** (805-862), content-negotiation refetch (870-919), a **full markdown pipeline** (921-1027), fast-mode bail, HTML handoff. Estimated cyclomatic complexity 70+.
**Task:** extract `fetchDocument(url, opts) (fetchOutcome, error)` (transport/policy, lines 360-698) into `fetch.go`-adjacent code; dispatch on content type to `extractPDF()` / `extractRawText()` / `extractNegotiatedMarkdown()` / existing `fromHTMLInternal`, each in its own file. Introduce a `fetchState`/`fetchOutcome` struct to replace the current 9-param stage functions with `*Result` out-params (`fetchWithRetryStage` at :195, `cacheLookupStage` returning 5 values at :122).

#### T11 — Extract the 4×-copied finalization + 4×/10×-copied post-processing helpers · **HIGH / M** (pairs with T10)
The ~25-field transport/telemetry tail (Status/timings/retries/redirects + `populateResponseHeaders` + `computeTraceInfo` + `fingerprintCDN` + …) is copy-pasted at `extract.go` 738-796 (PDF), 818-861 (raw), 962-1026 (markdown), 1053-1094 (HTML) — with drift already present (raw branch skips `HeadingCount`; only markdown handles `Link`/`X-LLMs-Txt`). The link-retention→dedupe→truncate→chunk sequence appears 4× (716-736, 808-816, 924-960, 1454-1538); the quality+fingerprint refresh trio appears **10×**.
**Task:** add `finalizeTransport(...)`, `applyContentPostProcessing(result, opts)`, and `refreshContentMetrics(result)`; call each once per branch. Also fold the content-negotiation refetch's inline re-implementation of decompress/charset (870-919) into the existing `decompressAndRestoreCharsetStage` (:316) — the copy silently drops error handling.

#### T12 — Decompose `fromHTMLInternal`'s 378-line fallback cascade · **HIGH / M**
`extract.go:1166-1543`: five stacked rescue heuristics (prune-fallback nested 6 deep at 1288-1336, preprocess-skip, index-page, JSON-LD articleBody, text fallback, LD-JSON supplement), each mutating overlapping subsets of `result` fields, so correctness depends on reading 380 lines of ordering.
**Task:** one function per rescue in `fallbacks.go` with the contract `applyXxxFallback(result *Result, ...) bool` (the already-extracted `applyPreprocessSkipFallback` at :1560 is the model); share one "adopt only if materially larger + refresh metrics" helper.

#### T13 — Decompose the `Result` god struct (234 fields) · **HIGH / L**
`result.go:9-360`: ~150 flat one-header-per-field copies (`ResponseXWixRequestId`, `ResponseXShopifyStage`, `ResponseXDenoRegion`, …) written by `populateResponseHeaders` (`headers.go:10`) and read back by `fingerprintCDN` (`cdn.go:40`); plus cache analysis, dedupe stats, and extraction payloads in one struct that every engine file touches. Each new vendor header grows the public JSON contract forever.
**Task:** restructure into embedded sub-structs (`TransportInfo`, `ResponseHeaders`, `CacheAnalysis`, `CDNInfo`, `TraceInfo`, `DedupeStats`) — Go embedding keeps the JSON wire format flat/unchanged; replace per-vendor string fields with a captured-headers `map[string]string` + the small derived set. Then `headers.go`/`cdn.go` become `http.Header → CDNInfo` functions instead of `*Result` mutations.

#### T14 — Split the `Options` god config; add ctx-first entry points · **MEDIUM / M**
`result.go:375-442`: 48 fields, 21 booleans of which 5 are negated (`NoNearDedupe`, `NoPruneFallback`, `NoCache`, `NoPDF`, `NoPooling` → double negation at call sites), config mixed with injected dependencies, and `Context context.Context` in the struct — the anti-pattern behind the ALP-043 class of bugs.
**Task:** add `FromURLContext(ctx, url, opts)` as the primary entry (facade included), deprecate `Options.Context`; group into `Fetch` / `Politeness` / `Cache` / `Extract` / `Output` sub-structs on the next minor; convert negative booleans to positive-with-default at the CLI parse layer. Move `Options` into its own `options.go`.

#### T15 — Deduplicate the fetch-setup path (`extract.go` vs `extract_head_only.go`) · **HIGH / M**
~80 lines copied nearly verbatim (`extract.go:412-581` vs `extract_head_only.go:51-133`): per-domain timeout, redirect tracker + security checker wiring, the 4-branch client-construction ladder, UA resolution, the byte-identical robots block, rate limiting. ALP-043-style fixes must currently be mirrored by hand — this is how drift happens.
**Task:** extract `buildFetchClient(opts, domain, tracker)`, `resolveUserAgentFor(opts, domain)`, `checkRobotsAllowed(...)`, `applyRateLimit(...)` into a shared `fetch_setup.go`; both entry points call them. (Subsumed by T10's `fetchDocument` if done together.)

#### T16 — Introduce interfaces at the real seams; delete mutable global test hooks · **MEDIUM / M**
`grep "interface {"` over non-test engine code returns **nothing**. Test seams are mutable package globals instead: `resolveHostIPs` (`security.go:79`), `testTLSConfig` (`utls.go:25`), `retryBackoffBase` (`extract.go:35`) — data races under `t.Parallel`, leak across tests. Time is concrete everywhere (`ratelimit.go`, `cache.go`, `extract.go`).
**Task:** add a `Resolver` (LookupIP) on `SecurityPolicy` and a `Clock`/ctx-aware `Sleeper` on the fetch pipeline; fold `testTLSConfig` into `chromeTransport` construction. Delete the three globals. Do **not** add more interfaces than these — current count is zero, overuse is not the risk here.

#### T17 — Preserve error identity at the API boundary · **MEDIUM / S**
32 `result.Error = err.Error()` assignments flatten well-designed sentinels (`security.go:68-75`, `scrape.go:16-24`) into strings; `FromURL*` return no `error`, so consumers (e.g. PinchTab) must string-match to distinguish "blocked by policy" from "timeout".
**Task:** add a typed `Result.Err() error` (non-serialized field preserving the sentinel chain) or an `ErrorKind` enum alongside the JSON string field.

#### T18 — Move the MCP tool layer out of `package main` · **MEDIUM / M**
All five tool definitions, JSON schemas, arg parsing, and guardrails (`maxScrapePages`, `cmd/seaportal/mcp.go:38-321`) live in the binary's main package — unreusable for an HTTP/SSE transport or PinchTab embedding, testable only through the binary.
**Task:** new `internal/mcp/tools` package importing the root `seaportal` facade (no cycle: root→engine, tools→root); `cmd/seaportal/mcp.go` shrinks to `runMCP`.

#### T19 — Unify fetch config across the 5 parallel option structs; single defaulting site · **MEDIUM / M**
`Options`, `ScrapeOptions`, `FetchBytesOptions`, `FlattenSitemapOptions`, `ParseFeedOptions` each re-declare `Timeout`/`UserAgent`/`Security`/`Client` subsets — T01 is the proof this drifts. Defaults are re-stated with *different values* across layers: engine retry defaults (`extract.go:487-494`) vs CLI (`main.go:183-185`: retries=3, 30s, 90s); sitemap depth/limit hardcoded in 3 places (`sitemap.go:59-61`, `main.go:37-38`, `mcp.go:166-168`); 30s timeout literals in 4+ files.
**Task:** one embedded `FetchConfig{Timeout, UserAgent, Security, Client}`; hoist magic numbers into named constants applied in exactly one `normalized()`-style function; CLI/MCP pass zero values and inherit library defaults.

#### T20 — Decompose `cmd/seaportal/main.go` `runExtract`; remove hidden filesystem write · **MEDIUM / M**
`main.go:167-532`: ~60 flag definitions, mutual-exclusion validation, stdin handling that mutates flags, an entire snapshot sub-program hidden behind `--snapshot` (337-372) sharing 60 irrelevant flags, a 500-char single-line `Options` literal (:391), four inline renderers — and the default markdown path **unconditionally writes** `renders/seaportal/<domain>_<ts>.{md,json}` into the user's CWD (497-520).
**Task:** extract `buildExtractOptions(fs)`, `renderResult(w, result, format)`, promote `--snapshot` to a real subcommand; gate the `renders/` write behind an explicit `--save-dir` flag.

#### T21 — Fix facade wrinkles: `var` exports, stale `ErrNotImplemented`, dead CLI branch · **LOW / S**
(a) `FlattenSitemap`/`ParseFeed` exported as reassignable `var` function values (`seaportal.go:262,273`) unlike every other wrapper. (b) `ErrNotImplemented` documented as "returned by ScrapeSite until the pipeline lands" but never returned anywhere (`scrape.go:59-69`). (c) `cmd/seaportal/scrape.go:99-119` renders partial results when `err != nil && res != nil`, but `ScrapeSite` never returns both non-nil — the dead branch hides that discovery-phase cancellation loses all partial output.
**Task:** convert vars to funcs; remove/deprecate `ErrNotImplemented`; either make `ScrapeSite` return `(partialResult, ctx.Err())` on cancellation or delete the branch.

#### T22 — Split the 66-file engine god package (do this LAST) · **MEDIUM / L**
One flat namespace spanning ~10 sub-domains (transport, politeness, security, cache, extraction, classification, text post-processing, output formats, crawling, snapshot). Verified low-coupling seams: `chunk.go`, `bm25.go`, `dedupe.go`, `snapshot.go` reference neither `Result` nor `Options`; the `internal/quality` extraction (bridged via type alias, `internal/engine/quality.go:6`) proves the pattern.
**Task:** split leaf-first, each step bridged with aliases: (1) `internal/textproc` (chunk, split, dedupe, bm25, citations, cleanup, markdown_truncate); (2) `engine/snapshot`; (3) `engine/politeness` (robots, ratelimit — after T03); (4) `engine/transport` (utls, http, fetch, security, useragents — after T10/T15). Requires T13/T14 first. Blockers to untangle on the way: `sleepCtx` (`extract.go:169`, used by ratelimit), `decompressBodyLimited`, `redirectTracker`.

### P2 — Duplication & code-level cleanups

#### T23 — Create `internal/engine/dom.go` with shared DOM/string primitives · **MEDIUM / S**
The HTML text-content walker exists **6×** (`index_extract.go:389`, `snapshot.go:518`, `schema.go:139`, `comments.go:382`, inline in `tables.go:~415` and `links.go:~115` — two are literal renames to dodge same-package collisions). Attribute lookup duplicated (`getAttr` / `snapshotGetAttr` identical 8-liners). Whitespace-collapse implemented **5 ways**, two of which compile a regexp **on every call** (`index_extract.go:404-408`, `dedupe.go:212-215` — also a perf bug).
**Task:** one `nodeText(n, skipScriptStyle)`, one `getAttr`/`hasAttr`, one allocation-free `collapseWhitespace` (adopt `links.go:148`'s); delete the variants.

#### T24 — seabench: shared report/stats/corpus helpers (~250 removable lines) · **MEDIUM / M**
(a) Six byte-identical JSON writers + the ~20-line emit block (MkdirAll → timestamp → write JSON+MD → println) repeated in all 8 subcommands. (b) Percentile/mean reimplemented 5× with **three different nearest-rank formulas** (`stress.go:230`, `sweep.go:416`, `tokens.go:225`, `eval.go:291`) — the divergence is itself a hazard; `mean` vs `meanFloat` duplicate justified by a false comment (`tokens.go:211`). (c) Corpus-fixture iteration copied in 4 lanes, with `eval.go` diverging on the synthetic base URL (`example.invalid` vs `corpus.local`), likely unintentionally.
**Task:** `report.go`: `emitReports(dir, cmd, report, markdown)`; `stats.go`: generic `percentile[T]` + one `mean`; `corpus.go`: `forEachFixture(path, fn)`.

#### T25 — Unify sanitize.go's twin single-pass scanners · **MEDIUM / S**
`removeAlwaysHiddenTagsSinglePass` (236-297) and `removeHiddenElementsSinglePass` (321-412) share a ~45-line identical skeleton, differing only in the removal predicate; a tokenizer bug (e.g. quote handling in `scanTagEnd`) must be found and fixed twice.
**Task:** one `removeElementsSinglePass(html, shouldRemove func(tag, attrs) bool)`; move tokenizer primitives to `html_scan.go`, keep policy tables in `sanitize.go`.

#### T26 — Deduplicate chunk.go heading anchors and snapshot.go traversal · **MEDIUM / S**
The anchor type + heading-attribution loop (a *documented deliberate rule*: H2–H6, H1 excluded) is encoded identically in `chunkBySentence` (471-496) and `chunkByWindow` (565-590). In `snapshot.go`, the inline root-traverse closure (54-76) duplicates `traverseInto` (133-154) including the interactive-filter branch — semantics can diverge between root children and everything below.
**Task:** extract `buildHeadingAnchors`/`headingAt`; delete the snapshot closure, call `ctx.traverseInto(doc, root, 0)`. Optionally split snapshot's ARIA vocabulary (273-498) into `aria.go`.

#### T27 — Small dedup batch · **LOW / S**
(a) Cached `http.Response` reconstruction: identical 7-line literal at `extract.go:132,142,632` → `cachedResponse.toHTTPResponse()`. (b) `defaultFetchUserAgent` (`fetch.go:10`) is a byte-identical private copy of `DefaultUserAgent` (`extract.go:74`) in the *same package* — a Chrome-version bump will miss one. (c) `cmd/seaportal/scrape.go:100-119` vs 124-141: same render switch → `renderScrapeResult()`. (d) MCP handler boilerplate: `requiredURLArg`/`securityFromArgs`/`marshalResult` helpers (`mcp.go`, ~30 lines). (e) `runSitemap`/`runFeed` near-twins (`main.go:34-118`).

#### T28 — Name classify.go's magic thresholds; kill the `isExtractClass` flag · **LOW / S**
`classifyPageInternal` (`classify.go:60-212`): threshold tuning — the file's whole purpose — requires hunting inline numbers (2000/500/800/300, confidence 80/50/60/30) across 350 lines; the `isExtractClass` boolean exists only to re-run the auth-wall check after the switch.
**Task:** package consts (`hydratedMinLength = 2000`, …); extract `classifyMediumConfidence(result)`; fold the auth-wall re-check in. Also convert `ComputeConfidence`'s 5 positional args incl. a bool (`detect.go:193`) to a small struct.

#### T29 — Route seabench through the facade; dead-code sweep · **LOW / S**
seabench imports `internal/engine` directly (e.g. `tokens.go:26`), freezing 73 exported engine funcs as a de-facto second API that a package split can't shrink. Dead code: `CrawlDelayCache.fetched` map allocated never used (`robots.go:19,46`); `_ = ogTitle` (`metadata.go:179`); `_ = headingLineConsumed` (`chunk.go:389`).
**Task:** switch seabench to the root `seaportal` package where the facade suffices; move `LoadCorpus` (`eval_corpus.go`) into `internal/testserver/fixture` or `internal/corpus`; one dead-code cleanup commit.

### P3 — Tests & CI

#### T30 — Wire the orphaned `integration` test lane into CI · **HIGH / S**
`//go:build integration` guards the full MCP JSON-RPC conformance suite (`cmd/seaportal/mcp_conformance_test.go`, 366 LOC), cold-start test (154 LOC), and the p95≤1.5s/p99≤2.5s latency gate (`internal/engine/latency_budget_test.go`). `grep -r integration` across `dev`, `scripts`, `.github/workflows` → **zero hits**. ~600 LOC of protocol and perf gates silently rot.
**Task:** CI step `go test -tags=integration ./cmd/seaportal/ ./internal/engine/ -run 'TestMCP|TestLatencyBudget'` (non-race lane) + a `dev test integration` entry.

#### T31 — Stop ignoring `testdata/**` in CI; fix e2e path filters · **HIGH / S**
`.github/workflows/ci.yml` lists `testdata/**` under `paths-ignore` — a commit that only edits a golden (byte-compared in `snapshot_golden_test.go:52-58`) skips the entire Go job and can land a red main. Separately, `e2e.yml` triggers on `cmd/**`, `pkg/**` (a directory that **does not exist**), `tests/e2e/**` — engine changes never trigger e2e on PRs.
**Task:** remove `testdata/**` from paths-ignore; replace `pkg/**` with `internal/**` and add `go.mod` to the e2e filter.

#### T32 — Wire allocs budget, fuzz, and npm tests into CI · **MEDIUM / S**
(a) `allocs_budget_test.go` (±15% B/op gate vs committed baseline) runs under `-tags=allocs` — no workflow uses the tag. (b) Five fuzz targets (`FuzzExtractFromHTML`, `FuzzSanitize`, …) only ever verify their seeds; the parsers ingest hostile web HTML — fuzzing's sweet spot. (c) `npm/tests/*.test.js` (platform detection = highest-blast-radius packaging code) run in no workflow; `reusable-release-publish.yml` publishes untested.
**Task:** CI step for `-tags=allocs` (advisory at first); weekly scheduled workflow `-fuzz=... -fuzztime=2m` per target committing crashers to `testdata/fuzz/`; `npm ci --ignore-scripts && npm test` step in CI and as a release precondition.

#### T33 — Add direct tests for the untested support-file cluster · **MEDIUM / M**
Zero test references for: `cdn.go` (229 LOC — `fingerprintCDN`/`parseViaHeader`), `headers.go` (217), `trace.go` (trace-ID parsers), `fetch.go` (`FetchBytes` — **exported**), `soft404.go`, `llmcontent.go`, `binary.go`, `http.go`'s `isRetryableError`/`addJitter`; plus `cmd/seabench/sweep.go` (575 LOC, largest seabench file). These are pure, trivially table-testable parsers — exactly where tests are cheapest. (The big three are fine: extract.go has 13 test files, chunk.go and snapshot.go have direct + golden coverage.)
**Task:** table-driven tests per file; one integration-style test for sweep. Pairs well with T04 (retryability rewrite needs the isRetryableError table anyway).

#### T34 — Inject a clock into `DiskCache`; kill real-sleep TTL tests · **MEDIUM / S**
`cache_test.go` holds 13 of the suite's 22 `time.Sleep` calls with margins as tight as 10ms-past-TTL (lines 334, 377, 419, 498, 583); `cache.go` uses `time.Now()` directly (191, 227). Classic loaded-CI flakes, amplified under `-race`.
**Task:** `nowFunc func() time.Time` field (same pattern as the existing `retryBackoffBase` hook); fake clock in tests. Folds into T16's Clock seam.

#### T35 — Shared test fixtures/server helpers; consolidation pass; `t.Helper` sweep · **LOW / M**
109 `httptest.NewServer` calls across 44 files with copy-pasted robots/sitemap/HTML handler boilerplate; 45 raw `os.ReadFile` fixture loads; `internal/testserver` imported by only 7 test files and does fragile cwd-guessing (`server.go:23-50`). 102 test files for 65 prod files with fragmentation (charset behavior spans 3 files; 25+ files under 60 LOC). 96 test helpers vs 47 `t.Helper()` calls.
**Task:** engine-local `loadFixture(t, name)` + `newSiteServer(t, pages)` helpers, migrate opportunistically; fold single-test micro-files into their subject's `_test.go`; mechanical `t.Helper()` sweep + enable the `thelper` linter in `.golangci.yml`.

#### T36 — Add a coverage floor · **LOW / S**
CI's "Coverage summary" step only echoes `go tool cover -func | tail -1` into the step summary — no gate, so T33's blind spots can decay silently while CI stays green.
**Task:** fail CI if total coverage drops below a committed floor (or Codecov project status, non-informational).

---

## Suggested sequencing

1. **Now (security/correctness, independent, mostly S/M):** T01, T02, T03, T30, T31 — then T04–T07.
2. **The big refactor arc (one initiative, in order):** T10 → T11 → T12 → T15 → T13 → T14 → T16. T10+T11 are effectively one change; do not attempt T22 before T13/T14.
3. **Parallel-friendly cleanups (any time):** T23–T29, T32–T36, T08, T09, T17–T21.
4. **Last:** T22 (package split) — only pays off after the god structs are decomposed; use the `internal/quality` alias-bridge pattern per step.

Counts: 36 tasks — 8 high, 17 medium, 11 low severity; ~14 are S-effort quick wins.
