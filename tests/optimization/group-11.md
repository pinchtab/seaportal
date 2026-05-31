# Group 11 — Search engines & site search

Search pages frequently render results client-side. The point of this group is to learn **which search endpoints are usable from seaportal** and which require a browser.

### 11.1 DuckDuckGo HTML
Fetch `https://html.duckduckgo.com/html/?q=hantavirus`. Report the first 5 result titles and URLs.
**Verify**: 5 result rows reported with hrefs; not a "no results" page.

### 11.2 Google (expected escalation)
Fetch `https://www.google.com/search?q=hantavirus`. Report outcome — pass = honestly flagged `blocked`/escalate; fail = pretends to have results.
**Verify**: `escalate` outcome OR a clear note that Google blocked the request.

### 11.3 Bing
Fetch `https://www.bing.com/search?q=hantavirus`. Report `pageClass` and whether organic result titles were extracted.
**Verify**: Honest verdict; if extracted, at least 3 result titles listed.

### 11.4 CNN site search (expected JS-only)
Fetch `https://edition.cnn.com/search?q=hantavirus` with `--probe-search --json`. Report whether actual hantavirus result titles came back and inspect `profile.outcome` + `profile.reasons`.
**Verify**: With `--probe-search` set, `profile.outcome == "needs-browser"` and `profile.reasons` contains `client-rendered-search` (the stable downstream signal). Without the flag, an honest escalate verdict still acceptable.

### 11.5 Wikipedia internal search
Fetch `https://en.wikipedia.org/w/index.php?search=hantavirus`. Report the first 5 article titles linked.
**Verify**: 5 article titles, all under `/wiki/`.

### 11.6 Repubblica internal search (SSR)
Fetch `https://www.repubblica.it/ricerca/?query=hantavirus`. Report the total result count shown and the first 5 headlines.
**Verify**: Both reported; count is a number; headlines are hantavirus-relevant.
