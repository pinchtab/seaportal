# Definition of Done (PR Checklist)

## Automated ✅ (CI enforces these)
These run automatically via `ci.yml`. If your PR fails them, fix and re-push.
- [ ] Go formatting & linting passes (gofmt, golangci-lint)
- [ ] Tests pass (`go test ./...`)
- [ ] Build succeeds (`go build ./...`)

## Manual — Code Quality (Required)
- [ ] **Error handling explicit** — All errors wrapped with `%w`, no silent failures
- [ ] **No regressions** — Extraction quality, confidence scoring, bot detection still work
- [ ] **SOLID principles** — Functions do one thing, testable, no unnecessary deps
- [ ] **No redundant comments** — Comments explain *why* or *context*, not *what*
  - ❌ Bad: `// Loop through items` above `for _, item := range items`
  - ✅ Good: `// AWS WAF returns empty challenge-container div`

## Manual — Testing (Required)
- [ ] **New/changed functionality has tests** — Unit tests in same package
- [ ] **Extraction changes tested** — Run against real URLs to verify quality
- [ ] **Detection changes tested** — Verify blocked/SPA detection still accurate

## Manual — Documentation (Required)
- [ ] **README.md updated** — If user-facing changes (API, options, output format)
- [ ] **docs/fixtures.md updated** — If new test fixtures added

## Manual — Review (Required)
- [ ] **PR description explains what + why**
- [ ] **Commits are atomic** — Logical grouping, good messages

## Conditional (Only if applicable)
- [ ] Breaking API changes documented in PR description

---

## Quick Checklist (Copy/Paste for PRs)
```markdown
## Definition of Done
- [ ] Tests added & passing
- [ ] Error handling explicit (wrapped with %w)
- [ ] No regressions in extraction quality
- [ ] No redundant comments (explain why, not what)
- [ ] README updated (if user-facing)
```
