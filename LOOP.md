# LOOP.md — Experimental Development Loop

An iterative loop where the React fixture grows harder and seaportal's experimental mode gets smarter.

## The Loop

### Step 1: Evolve the Fixture

- Review the last result in `results/` — compare it against what the page actually renders
- If the extraction captured all the content, make the fixture harder:
  - Add a small incremental feature (component, async content, dynamic text)
  - Add real sentences, paragraphs, meaningful content
  - Make it progressively harder for a plain HTTP fetch (no JS engine) to see the content
- Examples of complexity to add:
  - State-dependent rendering (content appears after interaction)
  - Async data loading (setTimeout, fetch simulation)
  - Conditional rendering based on JS logic
  - Dynamic text injection (dates, computed values)
  - Client-side routing
  - Lazy-loaded sections
  - Content behind tabs or accordions
- Keep changes small and incremental — one new thing per iteration

### Step 2: Verify the Fixture

- Start the fixture server: `./scripts/start-fixtures.sh`
- Open `http://localhost:8099/react-app.html` and verify the page renders correctly
- Confirm the new content is visible in the browser

### Step 3: Extract (Blind)

- Run seaportal experimental mode against the fixture — no peeking at the fixture source
- The goal: extract as much rendered content as possible without prior knowledge of the page
- Save the result:
  ```bash
  ./seaportal --experimental http://localhost:8099/react-app.html
  ```
- Compare the result against the actual page content
- If content is missing, improve `pkg/portal/experimental.go` to capture it

### Step 4: Commit and Repeat

- Commit both the fixture changes and any extraction improvements
- Push to `feat/experimental`
- Go back to Step 1
