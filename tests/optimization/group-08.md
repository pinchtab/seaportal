# Group 8 — Multi-hop navigation

These tasks require chaining multiple `seaportal` invocations. Track URLs to avoid loops. Bound: 6 hops max per task.

### 8.1 HN → comments
Fetch `https://news.ycombinator.com/`, pick the top story, follow its **comments** link (the "N comments" link, not the article), and report the top-voted comment's first sentence.
**Verify**: A comment text is reported from the right thread.

### 8.2 Wikipedia chain
Start at `https://en.wikipedia.org/wiki/HTTP`. From it, follow the link to "HTTPS", then from HTTPS follow the link to "Transport Layer Security". Report the URL of the final page and its first heading.
**Verify**: Final URL is the TLS article; heading reported.

### 8.3 Search → result → fetch
Use `https://html.duckduckgo.com/html/?q=site%3Apkg.go.dev+http.Client` to find a result, follow it, and report the function signature of `http.Client.Do` if present.
**Verify**: Either the signature is found, or honest "could not navigate" with hop count and reason.

### 8.4 News topic deep-dive
Start at `https://www.repubblica.it/ricerca/?query=hantavirus`. Pick the top result, fetch the article, then from that article follow one related-topic link. Report all three URLs and the final article's first paragraph.
**Verify**: 3 URLs in chain; final paragraph quoted. If the article body is paywalled and only an abstract is available, record `escalate-paywall` (not plain `escalate`) — the chain still counts as completed.

### 8.5 Pagination
From `https://www.repubblica.it/ricerca/?query=hantavirus` follow the link to page 2, then page 3. Report the count of result headlines on each page.
**Verify**: Three page URLs visited; per-page counts reported.
