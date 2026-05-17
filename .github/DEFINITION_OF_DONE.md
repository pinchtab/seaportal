# Definition of Done (PR Checklist)

## Automated ✅ (CI / pre-push gate enforces these)

Run automatically via `./dev all` (and CI). Fix and re-push on failure.
- [ ] Go formatting passes (gofmt)
- [ ] Static analysis passes (go vet, golangci-lint)
- [ ] Build succeeds (`go build ./...`)
- [ ] Unit tests pass (`go test ./...`)
- [ ] E2E tests pass (`./dev e2e`)
- [ ] Branch naming follows convention

## Manual — Code Quality (Required)

- [ ] **Error handling explicit** — All errors wrapped with `%w`, no silent
      failures. Cache / SWR background goroutines may swallow errors only
      with an inline comment explaining why.
- [ ] **No regressions** — If you touched extraction, run `./dev bench eval`
      and confirm seaportal's F1 didn't drop materially.
- [ ] **SOLID** — Functions do one thing, testable, no unnecessary deps.
- [ ] **No site-specific code** — Cross-cutting infrastructure (Cloudflare,
      Wikidata URL patterns) is acceptable; per-host code is not.
- [ ] **No redundant comments** — Comments explain *why* or *context*, not
      *what* the code does.
  - ❌ Bad: `// Loop through items` above `for _, item := range items`
  - ❌ Bad: `// Return error` above `return err`
  - ✅ Good: `// regression: wikidata-edit-property-pencil-leak — Wikipedia infobox edit pencils render as image-only links to Wikidata Q*#P* anchors`
  - ✅ Good: `// performance: dedupe disabled above 500 blocks; quadratic comparison`

## Manual — Testing (Required)

- [ ] **Bug fix lands with a regression test** — named `// regression:
      <kebab-slug>` per `CONTRIBUTING.md`. The test must fail on the
      pre-fix code (verify by reverting locally).
- [ ] **New functionality has tests** — Same-package unit tests preferred.
      Prefer minimal synthetic fixtures over scraped real-world HTML.
- [ ] **Docker E2E tests run locally** — if you modified the CLI subcommand
      dispatch, transport, or output format: `./dev e2e`.

## Manual — Documentation (Required)

- [ ] **`skills/seaportal/SKILL.md` updated** — if user-facing CLI flag,
      subcommand, or `pageClass` semantic changed.
- [ ] **`RELEASE.md` updated** — if release process or `.goreleaser.yml`
      structure changed.

## Manual — Review (Required)

- [ ] **PR description explains what + why** — especially classification
      or extraction-quality impact.
- [ ] **Commits are atomic** — logical grouping, good messages.

## Conditional (Only if applicable)

- [ ] **Capability suite re-run** — if classification, extraction, or
      preprocessing changed, run `./dev opt baseline` and confirm no
      category regresses.
- [ ] **Allocation budget** — if hot-path allocations changed, run
      `go test -tags=allocs ./internal/engine/` and update
      `tests/bench/profiles/allocs_baseline.json` if intentional.

---

## Quick Checklist (Copy/Paste for PRs)
```markdown
## Definition of Done
- [ ] `./dev all` passes (check + test + e2e)
- [ ] Bug fix has a `// regression: <slug>` test that fails pre-fix
- [ ] Error handling explicit (wrapped with %w)
- [ ] No site-specific code beyond cross-cutting CDN/parser patterns
- [ ] No redundant comments (explain why, not what)
- [ ] `skills/seaportal/SKILL.md` updated if user-facing
- [ ] `./dev bench eval` re-run if extraction touched
```
