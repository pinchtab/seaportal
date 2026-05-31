# Group selftest — Curated 10-task scoreboard

This file is the focused subset used by `seabench selftest` to produce a
quick scoreboard answer to "does an AI driving seaportal actually
succeed?". Tasks are cherry-picked from the larger `group-XX.md` corpus.

Each task declares an `**Expected escalation:** yes|no` line so the
scoreboard can compute escalation correctness. Use `yes` for tasks where
seaportal is expected to surface `needsBrowser=true`, a `spa`/`blocked`
class, or a paywall preview. Use `no` for tasks where seaportal should
return usable content without escalation.

Record outcomes with `./record.sh <step_id> pass|fail|escalate|escalate-paywall <note>`.

The selftest budget is **≤ 6 seaportal invocations per task**; if you
exceed that, record `fail` with a note explaining why.

---

### 1.1 Extract title from a static article
Fetch `https://example.com` and report the page title and `pageClass`.
**Verify**: title is "Example Domain", `pageClass` is `static`.
**Expected escalation:** no

### 1.2 Follow a link to a related Wikipedia article
Starting from `https://en.wikipedia.org/wiki/HTTP`, follow the link to "HTTPS" and report the first heading of the destination page.
**Verify**: Final URL is the HTTPS article; first heading reported.
**Expected escalation:** no

### 1.3 Identify pageClass=ssr and trust extraction
Fetch `https://en.wikipedia.org/wiki/Go_(programming_language)` as JSON. Report `pageClass`, `confidence`, and `validation.isValid`.
**Verify**: `pageClass` is `ssr` or `hydrated`; `validation.isValid` is true.
**Expected escalation:** no

### 1.4 Identify pageClass=blocked and escalate
Fetch `https://www.reuters.com/`. Record the outcome (`pass` if seaportal got real content, `escalate` if it correctly flagged blocked/browser-needed).
**Verify**: Outcome and `pageClass` recorded honestly.
**Expected escalation:** yes

### 1.5 Identify pageClass=spa and escalate
Run `seaportal --fast https://twitter.com/` and report the outcome. Do not retry.
**Verify**: Outcome is `escalate`; `pageClass` is one of `spa`/`dynamic`/`blocked`, OR `needsBrowser` is true.
**Expected escalation:** yes

### 1.6 Accessibility snapshot of interactive elements
For `https://news.ycombinator.com/`, get the interactive compact snapshot and report how many `link` rows it contains.
**Verify**: A non-zero number of links is reported; no full Markdown was fetched in this step.
**Expected escalation:** no

### 1.7 Multi-site comparison
Fetch `https://text.npr.org/` and `https://lite.cnn.com/`. Report `pageClass` for each and the count of distinct article links found on each page.
**Verify**: Both extracted; non-zero link counts; both classify as static or ssr.
**Expected escalation:** no

### 1.8 Multi-hop navigation (≤ 3 hops)
Start at `https://en.wikipedia.org/wiki/HTTP`. Follow the link to "HTTPS", then from HTTPS follow the link to "Transport Layer Security". Report the URL of the final page and its first heading.
**Verify**: Final URL is the TLS article; first heading reported.
**Expected escalation:** no

### 1.9 CJK content extraction without mojibake
Fetch `https://ja.wikipedia.org/wiki/日本`. Report the first heading and the first sentence verbatim. Confirm Japanese characters survived round-trip.
**Verify**: Heading and sentence contain CJK characters; no mojibake.
**Expected escalation:** no

### 1.10 Sitemap or feed subcommand smoke
Fetch `https://danluu.com/`. Report `pageClass` and list five post titles from the index.
**Verify**: `pageClass` is `static`; five titles reported.
**Expected escalation:** no
