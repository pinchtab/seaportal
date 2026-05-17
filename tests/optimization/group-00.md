# Group 0 — Sanity & Classification

### 0.1 Static page
Fetch `https://example.com` and report the page title and `pageClass`.
**Verify**: title is "Example Domain", `pageClass` is `static`.

### 0.2 Classification of a known SSR site
Fetch `https://en.wikipedia.org/wiki/HTTP` as JSON. Report `pageClass`, `confidence`, and `validation.isValid`.
**Verify**: `pageClass` is `ssr` or `hydrated`; `validation.isValid` is true.

### 0.3 Detect a SPA without fetching content
Run `seaportal --fast https://twitter.com/` and report the outcome. Do not retry.
**Verify**: Outcome is `escalate`; `pageClass` is one of `spa`/`dynamic`/`blocked`, OR `needsBrowser` is true.

### 0.4 Snapshot vs Markdown trade-off
For `https://news.ycombinator.com/`, get the **interactive compact snapshot** and report how many `link` rows it contains.
**Verify**: A non-zero number of links is reported; no full Markdown was fetched in this step.

### 0.5 Confidence is honest
Fetch a deliberately ambiguous page (e.g. `https://www.iana.org/`) as JSON. Report `pageClass`, `confidence`, and `classReasons`. Comment on whether the confidence value matches your intuitive read of the page.
**Verify**: All three fields reported; brief judgment recorded.

### 0.6 `--fast` actually saves work
Fetch `https://www.airbnb.com/` once with `--fast` and once without. Report the elapsed time printed in the saved-file line for each run.
**Verify**: `--fast` finishes at least 1.5× faster than the full fetch (e.g. ~0.9 s fast vs ~2.0 s full is a typical reading; allow for network jitter — what matters is the ratio, not absolute milliseconds). Do **not** require non-zero content length: airbnb's SPA shell returns near-empty content in either mode, and the point of this step is to verify the `--fast` bail-out, not extraction quality.
