# Testing & Coverage Report — seaportal

- **Date:** 2026-07-12 · branch `quality/audit-fixes` (post-audit)
- **What this is:** the result of running every test lane the project has, plus an honest map of what is and isn't covered. Companion to `audit-code-review.md`.

---

## 1. Battery results — every lane, run to completion

| Lane | Command | Result |
|---|---|---|
| Format | `gofmt -l` | ✅ clean |
| Vet | `go vet ./...` | ✅ clean |
| Lint | `golangci-lint run ./...` | ✅ **0 issues** (errcheck, govet, staticcheck, unused, ineffassign, misspell, unconvert, **thelper** now enabled) |
| Unit + race | `go test ./... -race -covermode=atomic` | ✅ all packages pass |
| Integration (MCP) | `go test -tags=integration -run TestMCP` | ✅ conformance suite + cold-start (median 8.5 ms) |
| Latency budget | `go test -tags=integration -run TestLatencyBudget` | ⚠️ **advisory** — p95 ≈ 1.55 s vs 1.5 s target; exceeded on pre-audit `main` too (multi-MB Wikipedia fixtures). Now a non-blocking CI step. |
| Allocation budget | `go test -tags=allocs -run TestAllocationBudgets` | ✅ all 7 hot-path benchmarks within ±15% of baseline |
| Fuzz | `./dev fuzz` (9 targets) | ✅ no crashers; ~20–40k execs/s per target |
| npm wrapper | `npm ci && npm test` | ✅ **27/27** |
| Docker e2e | `tests/e2e/run-local.sh` | ✅ **106/106 checks** across 11 scenarios (baseline `main` was partially red) |
| Benchmarks | `seabench eval/classify/stress/cachebench/tokens` | ✅ ran — classify **39/40 (0.975)**, eval F1 **0.763**, stress 157 urls/s @ 100%, cache hit 84% |

Total statement coverage: **80.9%** (floor gate committed at 78).

### Two gates that had never actually run
Wiring the orphaned lanes into CI (audit T30/T32) immediately caught two latent failures the "green" build was hiding:
1. `TestLatencyBudget` **panicked** on a duplicate ServeMux route — the eval corpus lists some fixture paths twice. Fixed by deduping route registration.
2. `./dev fuzz` matched targets with `grep -P`, which **doesn't exist on macOS BSD grep** — so the local fuzz lane silently ran **zero** targets. Fixed with `grep -E` + `sed`.

Neither would have been found without turning the gates on. That is the core lesson of this pass: a test that never runs is not coverage.

---

## 2. What we are properly testing

**Core extraction engine (`internal/engine`, 88.2%)** — the crown jewel. Pure transforms are tested directly, not just end-to-end:
- **Chunking, dedupe, BM25, sanitize, cleanup, markdown-truncate** — direct table-driven tests; property-style fuzzing on the hostile-HTML entry points.
- **Classification** — a 40-fixture accuracy corpus (`seabench classify` = 97.5%) plus a table-driven audit suite.
- **Charset/encoding** — dedicated recovery + negotiation tests with regenerable fixtures (`scripts/gen-charset-fixtures.go`).
- **Snapshot (accessibility tree)** — byte-compared golden files with a documented `UPDATE_GOLDEN=1` regen path.
- **Security/SSRF** — the redirect-revalidation, private-IP, decompression-bomb, and size-cap paths are exercised with an injectable resolver seam.
- **Newly covered this pass** (audit T33 — previously untested support files): `cdn.go` (provider fingerprints, Via parsing), `trace.go` (W3C/B3 trace IDs), `headers.go` (`ResponseHeaders.populate`), `fetch.go` (`FetchBytes` happy/blocked/capped/cancelled), `soft404.go`, `llmcontent.go`, `binary.go` content-type table, and the retryability classifier (`http.go`).

**JSON wire contract** — a new golden (`result_json_stability_test.go`) locks the exact serialization of the 234-field `Result` (key order + omitempty), so the struct decomposition into embedded sub-structs is provably byte-identical to the pre-refactor output. This is the safety net that made the biggest refactor safe.

**MCP protocol** — full JSON-RPC conformance + cold-start budget (now running in CI), plus new unit tests for the extracted tool layer (arg parsing, tool registration, error mapping).

**Concurrency** — the whole suite runs under `-race`; multi-MB fixtures are tag-routed out of the race lane (`skipHeavyFixture`) into a dedicated non-race step. New leak tests (`internal/engine/leakcheck`) assert the h2 connection cache doesn't leak goroutines/fds — the exact regression the audit's #1 finding fixed.

**End-to-end** — 11 Docker scenarios exercise the real binary against nginx: extraction formats, snapshot, and the full scrape pipeline (sitemap discovery, sitemap-index recursion, pattern grouping, sampling strategies, robots filtering, crawl fallback, and all three output shapes).

**Determinism & performance regression** — allocation budgets (±15% vs committed baseline), scrape determinism (identical sampling across runs), and the latency envelope are all encoded as tests.

---

## 3. What we are NOT covering (the honest gaps)

Ranked by how much the gap should worry you.

### Should address next
1. **CLI command layer (`cmd/seaportal`, 0.7% unit coverage).** This number is misleading but points at a real gap: most `cmd` tests spawn the binary as a subprocess (`main_*_test.go`), so their exercise doesn't attribute to coverage. The newly-decomposed pure helpers (`buildExtractOptions`, `renderResult`, `renderMarkdown`) are now unit-testable but have **no direct unit tests** — they're only hit via subprocess and e2e. Adding in-process tests for the option-building and rendering functions would convert ~500 lines from "tested by side effect" to "tested".
2. **MCP tool handler bodies (`internal/mcp/tools`, 36.3%).** The arg helpers and registration are unit-tested; the handler *bodies* (which perform real fetches) are covered only through the integration conformance suite, not in isolation. A fake-fetch seam would let each tool's success/error branches be tested directly.
3. **`chunk_config.go` (43%) and `text_fallback.go` (64%).** Config-parse error branches and the last-resort text-extraction fallback are under-exercised — both are pure and cheap to table-test.
4. **uTLS transport (`utls.go`, 67%).** The new h2 connection-cache eviction/redial paths (GOAWAY, dead-conn, LRU cap) have leak tests but the *error* branches (dial failure mid-cache, proxy failure) are thin. Hard to test without a fault-injecting TLS server.

### Structurally hard / accepted
5. **PDF extraction (`pdf.go`, 71%)** — depends on the third-party `ledongthuc/pdf` decoder; malformed-PDF branches are seeded by fuzz but not enumerated.
6. **Root facade (`seaportal` package, 5.7%)** — this is ~280 lines of one-line alias wrappers delegating to `internal/engine`. It's covered transitively; direct tests would only assert "the alias points where it says." **Low value — accept as-is.**
7. **Real-network paths** — everything runs against `httptest`/nginx fixtures (correct for determinism). No test hits a real external site; live-web brittleness (a CDN changing headers, a real robots.txt) is only caught by the hourly dogfood scrape loop, not CI.
8. **Latency budget** is advisory, not enforced — the p95 target (1.5 s) is currently unmet by the 2 MB multilingual fixtures. **Open decision:** either recalibrate the envelope to reality (~1.7 s) or optimize the large-page extraction path. Until then a genuine latency regression on *small* pages would still slip through (only p95/p99 are gated).

### Coverage-infra gaps (now partly closed)
9. **Fuzzing depth** — targets exist and now run weekly in CI at 2 min each (audit T32), but there's no committed corpus beyond the seeds, so accumulated coverage resets each run. Persisting `testdata/fuzz/` corpus across runs would deepen it.
10. **No mutation testing** — line coverage says a line *ran*, not that an assertion would catch it breaking. The 80.9% is honest line coverage; effectiveness is unmeasured.

---

## 4. One-line takeaway

The engine — where the risk lives — is genuinely well tested (88%, direct unit tests, goldens, fuzz, race, leak checks), and this pass both raised it and turned on three quality gates that were silently dead. The real remaining gap is the **edges**: the CLI and MCP handler bodies are validated end-to-end but not in isolation, so a logic bug in option-building or a tool handler would be caught by e2e (slow, coarse) rather than a unit test (fast, precise). That's the highest-value place to invest next.
