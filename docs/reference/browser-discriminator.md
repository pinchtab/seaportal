# Discriminator

SeaPortal is the first-pass fetcher for agents that want to avoid launching a
browser unless rendering can materially improve the result. This contract defines
the routing decision SeaPortal exposes so PinchTab can decide whether the
SeaPortal result is good enough or whether it should fall through to Chrome.

The decision is **merged into the existing `profile` object** rather than a
separate top-level field: `profile` already carries `outcome`, `reasons`,
`confidence`, and `trustworthy`, which a standalone discriminator would only
duplicate. Two fields are added — `decision` and `browserRecommended`.

## Goals

- Give PinchTab one explicit routing decision (`profile.decision`) plus one
  boolean to branch on (`profile.browserRecommended`) instead of forcing it to
  reverse-engineer SeaPortal scoring.
- Keep the raw `profile` fields (`outcome`, `reasons`, `confidence`,
  `trustworthy`) authoritative for debugging, metrics, and tuning.
- Distinguish "Chrome may help" from "the target is unreachable or not a useful
  browser benchmark".
- Avoid treating `quality` as a standalone truth signal (see Routing Inputs).

## Non-Goals

- SeaPortal does not launch Chrome.
- SeaPortal does not solve bot challenges.
- SeaPortal does not decide whether an agent task is allowed to use a browser.
  It only reports whether a browser is likely to improve content access.

## JSON Shape

`profile` in `--json` output gains `decision` and `browserRecommended`:

```json
{
  "url": "https://example.com",
  "statusCode": 200,
  "pageClass": "ssr",
  "length": 3001,
  "isSpa": false,
  "isBlocked": false,
  "profile": {
    "class": "ssr",
    "outcome": "extract",
    "decision": "static-high-confidence",
    "browserRecommended": false,
    "reasons": ["high-confidence", "ssr-markers-present"],
    "confidence": 100,
    "trustworthy": true
  }
}
```

Go callers read `result.Profile.Decision` / `result.Profile.BrowserRecommended`
(type `seaportal.BrowserDecision` + the `seaportal.Decision*` constants).

## Field Contract

| Field | Type | Meaning |
| --- | --- | --- |
| `profile.decision` | string enum | Stable routing category for callers. |
| `profile.browserRecommended` | boolean | `true` when PinchTab should use Chrome for a content/resource check. |
| `profile.outcome` | string enum | Existing coarse outcome the decision refines (`extract` / `extract-with-warning` / `needs-browser` / `fail-fast`). |
| `profile.reasons` | string array | Existing contributing signals for metrics and audit (serves the role of the old `signals`). |
| `profile.confidence` | int 0–100 | Existing extraction confidence (NOT decision confidence). |
| `profile.trustworthy` | boolean | Existing trust flag derived from confidence. |

There is no separate `confidence` enum for the decision: callers derive a
high/medium/low view from `trustworthy` + `confidence` if needed.

## Decision Enum

| Decision | `browserRecommended` | Meaning |
| --- | ---: | --- |
| `static-high-confidence` | false | Strong SeaPortal result; safe to skip Chrome unless the task explicitly needs rendering, screenshots, cookies, or interaction. |
| `static-ok` | false | Good enough for normal read/resource-check tasks. |
| `static-caution` | false | Usable content but warning/dynamic signals (or an intentionally thin page). PinchTab may escalate when the caller asks for completeness. |
| `browser-needed` | true | Browser rendering is likely to materially improve the content check. |
| `blocked` | true | Bot protection, captcha, access-denied, or auth-wall. Chrome may still need auth/challenge policy. |
| `unreachable` | false | DNS, TLS, connection, timeout, or similar transport failure. Do not spend Chrome by default. |
| `not-found` | false | HTTP 404 or soft-404. Do not spend Chrome by default. |
| `unsupported` | false | Resource type SeaPortal cannot evaluate (binary-only content). |

## Routing Inputs

The decision is derived from the resolved `profile` plus these `Result` fields:
`statusCode`, `error`, `isBlocked`, `isSoft404`, `responseContentType`,
`length`, and the profile's `class` / `outcome` / `trustworthy`.

**`quality` is deliberately not a gate.** SeaPortal's `quality` float is too
noisy to route on: excellent server-rendered pages routinely score near zero
(Wikipedia and GitHub score 0; Cloudflare ~16) despite clean, complete
extraction. The static tiers therefore key on `outcome` + `class` +
`trustworthy` + `length`, which track real extraction success far better. This
is a deliberate deviation from an earlier quality-threshold draft.

## Decision Rules

Evaluated in order; first match wins.

1. **unsupported** — `error` starts with "skipped binary content", or
   `responseContentType` is a known binary type.
2. **unreachable** — `error` is set and `statusCode == 0` (no usable HTTP
   response: DNS / TLS / connection / timeout).
3. **not-found** — `statusCode == 404`, `isSoft404`, or `reasons` contains
   `http-404-not-found`.
4. **blocked** — `isBlocked` or `class == blocked`.
5. By `outcome`:
   - `needs-browser` / `fail-fast` → **browser-needed**. This dominates length
     and quality: an auth-wall or SPA shell can carry many bytes and still
     require a browser.
   - `extract`:
     - `staticClass` (`static`/`ssr`/`hydrated`) AND `trustworthy` AND
       `length >= 1000` AND 2xx-or-no-status → **static-high-confidence**
     - else `length >= 500` AND 2xx-or-no-status → **static-ok**
     - else → **static-caution** (extract succeeded but the body is thin,
       including intentionally minimal pages; the classifier trusted it, so we
       don't spend a browser)
   - `extract-with-warning`:
     - `length >= 500` → **static-caution**
     - else → **browser-needed**

## PinchTab Consumption Policy

| Decision | Default PinchTab action |
| --- | --- |
| `static-high-confidence` | Use SeaPortal result; skip Chrome. |
| `static-ok` | Use SeaPortal result; skip Chrome for read/resource checks. |
| `static-caution` | Use SeaPortal for best-effort reads; use Chrome for completeness-sensitive checks. |
| `browser-needed` | Use Chrome if the caller requested a resource/content check and browser use is allowed. |
| `blocked` | Use Chrome only under explicit challenge/auth/manual policy. |
| `unreachable` | Do not launch Chrome by default; report transport failure. |
| `not-found` | Do not launch Chrome by default; report not found. |
| `unsupported` | Use caller-specific fallback, not automatic Chrome. |

PinchTab must still override SeaPortal for task shape. Screenshots, PDFs,
interaction, cookies, session state, and browser network inspection require
Chrome even when the decision is `static-high-confidence`.

## Compatibility

- `outcome` is retained and unchanged; `decision` rides alongside it (it is not
  a replacement, and existing consumers of `outcome`/`reasons` keep working).
- Adding fields to `profile` is backward-compatible for JSON consumers.
- New enum values may be added later. Callers should treat an unknown decision
  as `browser-needed` only when `browserRecommended == true`; otherwise as
  low-confidence `static-caution`.

## Acceptance Tests

Covered by `TestDeriveDecision` in `internal/engine/discriminator_test.go`:
static-high-confidence (SSR + FromHTML), static-ok, static-caution (thin and
warning-shell), browser-needed (SPA + auth-wall dominance), blocked,
unreachable, not-found (hard + soft), and unsupported (binary).
