# LOOP.md — Experimental Development Loop

Two-agent loop: one evolves the fixture, the other extracts blind.

## Agent A: Fixture Evolver (sub-agent)

- Evolves `tests/e2e/fixtures/react-app.html` each iteration
- Adds one small incremental feature making content harder to extract without JS
- Adds real sentences, paragraphs, meaningful content
- Does NOT touch seaportal source code
- Commits fixture changes and pushes
- Reports back only: "fixture updated" (no details about what changed)

Examples of complexity to add:
- Async data loading (setTimeout, fetch simulation)
- Computed/derived content from JS logic
- Content behind tabs, accordions, modals
- IntersectionObserver / lazy-loaded sections
- Client-side routing with hash fragments
- Dynamic text injection (dates, computed values)
- Content assembled from multiple data sources
- State-dependent rendering

## Agent B: Extractor (main agent)

- Runs `./seaportal --experimental <url>` against the fixture
- Does NOT read the fixture source code
- Compares extraction result against what seaportal captures
- If content seems incomplete, improves `pkg/portal/experimental.go`
- Commits extraction improvements and results
- Uses only seaportal output + browser snapshot to evaluate quality

## The Loop

1. **Agent A** evolves the fixture, commits, reports "fixture updated"
2. **Agent B** extracts blind, saves result, evaluates completeness
3. **Agent B** checks if extraction captured everything meaningful
4. If gaps found: **Agent B** improves experimental.go
5. Commit results and repeat

## Quality Check

Agent B can run `--experimental --snapshot` to get the accessibility tree and compare element count / structure against the extracted markdown to detect missing content.
