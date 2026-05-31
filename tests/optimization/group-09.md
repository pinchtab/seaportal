# Group 9 — Edge cases & resilience

### 9.1 RFC plaintext
Fetch `https://www.rfc-editor.org/rfc/rfc2616`. Report the document title and section 1 heading.
**Verify**: Both reported correctly.

### 9.2 httpbin HTML demo
Fetch `https://httpbin.org/html`. Report the first paragraph's opening words.
**Verify**: Recognizable Moby-Dick prose.

### 9.3 RSS XML
Fetch `https://feeds.bbci.co.uk/news/rss.xml`. Report the outcome — does seaportal handle XML feeds gracefully?
**Verify**: Honest outcome; either parsed-ish content or a clear "cannot extract" signal.

### 9.4 Token-bounded snapshot
Fetch the snapshot of `https://en.wikipedia.org/wiki/Computer` with `--max-tokens=1500`. Report the approximate number of nodes in the truncated tree.
**Verify**: A number is reported; output is smaller than the unbounded version.

### 9.5 Compact-vs-JSON snapshot size
For `https://example.com`, compare the byte size of `--snapshot --format=json` vs `--snapshot --format=compact`.
**Verify**: Both sizes reported; compact is smaller.

### 9.6 Redirect chain
Fetch `http://github.com` (note: HTTP, not HTTPS). Report the final URL after redirects and confirm the page extracted.
**Verify**: Final URL reported; extraction succeeded.

### 9.7 404 handling
Fetch `https://www.bbc.co.uk/news/this-page-definitely-does-not-exist-xyz123`. Report the outcome — does seaportal report a clean failure or hallucinate content?
**Verify**: Honest 404 / not-found signal; no fabricated body content.

### 9.8 Very large page
Fetch `https://en.wikipedia.org/wiki/List_of_Latin_phrases_(full)` and report `length`. Then fetch with `--max-tokens=2000` and confirm the truncated version is smaller.
**Verify**: Two `length` values; truncated is smaller.
