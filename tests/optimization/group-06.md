# Group 6 — SPA / blocked (correct escalation)

These tasks are designed to **fail** at extraction. Pass = seaportal correctly signals "needs browser". A pass here is reported as outcome `escalate`.

### 6.1 Twitter
Run `seaportal --fast https://twitter.com/`. Was escalation signaled?
**Verify**: `escalate` outcome with `pageClass` ∈ {spa, dynamic, blocked} or `needsBrowser: true`.

### 6.2 Instagram
Same for `https://www.instagram.com/`.
**Verify**: `escalate` outcome.

### 6.3 LinkedIn
Same for `https://www.linkedin.com/`.
**Verify**: `escalate` outcome.

### 6.4 False negative check
Run on `https://www.airbnb.com/` — heavily hydrated, may or may not extract. Honestly report whether content came back usable.
**Verify**: Outcome reflects reality.

### 6.5 Login wall
Run on `https://www.facebook.com/`. Verify outcome is escalate; report whether seaportal labels it `blocked` or `spa`.
**Verify**: Escalate outcome with class recorded.

### 6.6 Cloudflare-protected
Run on `https://nowsecure.nl/` (well-known Cloudflare challenge demo). Verify seaportal flags it correctly rather than claiming success.
**Verify**: `blocked` class OR escalation; no false claim of useful content.

### 6.7 Login-wall content assertion
Run on `https://www.linkedin.com/` AND on a non-LinkedIn auth-walled host
(e.g. `https://medium.com/@some-user/some-member-only-post` or
`https://www.threads.net/`) and inspect `profile.reasons` for each. The
classifier must explicitly tag the page so downstream agents escalate even
when class alone (`ssr`) would look usable.
**Verify**: `profile.outcome == needs-browser` AND `profile.reasons` contains
EITHER `auth-wall-content` (current, content-driven detector) OR
`auth-wall-marketing` (legacy host-gated detector — accepted during the
migration window so agents running mid-flight don't regress). Both reasons
denote the same finding; downstream tools should migrate to
`auth-wall-content`. A pass on outcome alone is **not** sufficient — this
sub-task catches regressions where a future heuristic flips the outcome via
a different code path and silently drops the auth-wall marker. The non-
LinkedIn run also confirms that the detector is no longer gated by a fixed
host list. Equivalent assertion applies to `https://www.facebook.com/` when
it returns class `ssr` instead of `spa`.
