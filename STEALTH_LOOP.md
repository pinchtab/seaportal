# STEALTH_LOOP.md — Adversarial Bot Detection Loop

Two-agent adversarial loop: one hardens detection, one bypasses it.

## Overview

```
Agent A (Defender): Adds bot detection checks to fixture
Agent B (Attacker): Tries to bypass detection with --escape-detection flag
```

Goal: Build robust stealth capabilities by evolving against real detection techniques.

## Agent A: Detection Evolver (sub-agent)

- Evolves `tests/e2e/fixtures/bot-detection.html`
- Adds ONE new detection technique per iteration
- Does NOT touch seaportal source code
- Commits with `detection(N): brief description`
- Reports ONLY: "detection updated" (no details)

**Detection techniques to add:**
1. ✅ navigator.webdriver check
2. ✅ CDP marker detection (cdc_*)
3. ✅ Plugins array emptiness
4. ✅ Languages array check
5. ✅ WebGL software renderer (SwiftShader)
6. ✅ Screen dimension anomalies
7. ✅ Notification permission state
8. ✅ HeadlessChrome in user agent
9. Canvas fingerprint consistency
10. AudioContext fingerprint
11. Performance.now() precision (< 100μs = automation)
12. Mouse movement entropy
13. Keyboard timing patterns
14. Font enumeration
15. Battery API state
16. Connection API check
17. Permissions API probing
18. Chrome runtime object inspection
19. Stack trace inspection (puppeteer/playwright signatures)
20. setTimeout/setInterval override detection

## Agent B: Stealth Evolver (main agent)

- Runs `./seaportal --escape-detection <url>`
- Checks if protected content is extracted
- If blocked: improves stealth in `pkg/portal/stealth.go`
- Commits fixes and results
- Uses only extraction output to evaluate success

**Stealth techniques from analysis:**
- fingerprint-generator/injector patterns (steel-browser)
- Patchright stealth flags (scrapling)
- cdc_* marker deletion (steel-browser)
- navigator.webdriver = false
- Mock chrome.runtime object
- Canvas noise injection
- WebGL parameter spoofing
- Plugin/language array injection
- User agent normalization

## The Loop

1. **Agent A** adds detection check, commits, reports "detection updated"
2. **Agent B** extracts with `--escape-detection http://localhost:8765/bot-detection.html`
3. **Agent B** checks: was "Protected Content Unlocked" captured?
4. If NO: **Agent B** adds stealth bypass, re-extracts, verifies
5. Commit results and repeat

## Success Criteria

- All Wave 1-4 checks show ✅ (currently 17 total)
- Diamond tier achieved (450+ pts → "GHOST IN THE MACHINE")
- Extract contains tiered secrets (Bronze → Silver → Gold → Diamond)

## Commands

```bash
# Start fixture server
cd ~/dev/seaportal/tests/e2e/fixtures && python3 -m http.server 8765

# Run stealth extraction
./seaportal --escape-detection http://localhost:8765/bot-detection.html

# Check results
grep "Protected Content" results/*.md | tail -1
```

## Cron Schedule

Runs hourly via OpenClaw cron, offset from LOOP.md extraction loop.
