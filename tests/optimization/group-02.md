# Group 2 — News sites (variable rendering)

### 2.1 Text-mode mirrors
Fetch `https://text.npr.org/` and `https://lite.cnn.com/`. Report `pageClass` for each and the count of distinct article links you can find on each page.
**Verify**: Both extracted; non-zero link counts; both classify as static or ssr.

### 2.2 Hacker News front page
Fetch `https://news.ycombinator.com/` and list the titles of the top 5 stories.
**Verify**: Five titles reported, in order.

### 2.3 Heavy news site
Fetch `https://www.bbc.com/news`. Report whether seaportal extracted usable content (judge by `length` and `validation.isValid`) or whether you would escalate.
**Verify**: A clear "use" or "escalate" verdict with the supporting fields.

### 2.4 Likely-blocked site
Fetch `https://www.reuters.com/`. Record the outcome (`pass` if seaportal got real content, `escalate` if it correctly flagged blocked / browser-needed).
**Verify**: Outcome and `pageClass` are recorded honestly.

### 2.5 Italian news homepage
Fetch `https://www.corriere.it/` and `https://www.repubblica.it/`. Report 5 top headlines from each.
**Verify**: 5 headlines from each, in Italian, recognizable as today's news.

### 2.6 Headline dedup honesty
Fetch `https://www.repubblica.it/` as JSON. Report `dedupeApplied`, `duplicatesRemoved`, and `duplicateSignals`. Sanity-check the count against the visible duplication in the rendered Markdown.
**Verify**: All three fields reported; judgment recorded on whether dedup looks sensible.
