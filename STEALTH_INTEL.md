# Stealth Intelligence — Advanced Bot Detection (2026)

## Context
The basic 8 checks (webdriver, CDP markers, plugins, languages, WebGL renderer, screen, notifications, UA) are "table stakes" — trivially bypassed by puppeteer-extra-stealth. Enterprise sites use defense-in-depth with ML scoring.

---

## Detection Layers

### 1. Network & Protocol Fingerprinting (Pre-JS, Server-Side)
- **TLS/JA3/JA4 fingerprinting**: Cipher suites, extensions, order, ALPN
- **HTTP/2 frame analysis**: Stream multiplexing, header ordering, Client Hints (sec-ch-ua), WINDOW_UPDATE behavior
- **Mitigation**: uTLS, custom network stacks

### 2. Advanced Browser Fingerprinting
- **Canvas fingerprinting**: Pixel-level hash of specific drawing operations
- **AudioContext fingerprinting**: Audio processing quirks
- **Full WebGL**: Extensions, shaders, rendering fidelity (not just renderer string)
- **Font enumeration**: Rendering tests
- **Hardware signals**: hardwareConcurrency, deviceMemory, motion/orientation sensors, battery
- **Consistency correlation**: ML checks if attributes match believable device profile
- **Tampering detection**: Property descriptor inspection catches `Object.defineProperty` spoofs

### 3. Behavioral Biometrics (The "Killer" Layer)
- **Mouse trajectories**: Bezier curves, acceleration, micro-pauses vs linear paths
- **Scroll velocity/momentum**: Natural deceleration patterns
- **Typing cadence**: Key press intervals, corrections
- **Session patterns**: Navigation timing, hesitation, interaction flow
- **ML models**: Real-time scoring on these signals

### 4. Dynamic JavaScript Challenges
- **Proof-of-work**: CPU tasks, memory allocation probes
- **Rendering probes**: Invisible micro-challenges
- **Execution context inspection**: Stack traces, CDP detection, proxy object detection
- **Signed cookies**: cf_clearance, _abck, _px — IP/UA-bound, non-replayable

### 5. AI/ML Risk Scoring + Commercial Platforms
Combines all signals with:
- IP/ASN reputation
- Request cadence
- Header entropy
- Historical behavior

**2026 Leaders:**
| Provider | Focus |
|----------|-------|
| Cloudflare Bot Management | Turnstile + edge ML (most widespread) |
| DataDome | Real-time behavioral ML + slider challenges |
| Akamai Bot Manager | Sensor data + TLS fingerprinting |
| HUMAN (PerimeterX) | Behavioral + proof-of-work |
| Imperva, Kasada | Strong behavioral focus |

**Honeypots**: Hidden form fields, AI-generated fake pages, garbage data to waste scraper resources.

---

## Why Current Stealth Won't Hold

"Stealth Loop PASS" defeats 2018–2022 era checks. Modern systems:
- Assume basics are spoofed
- Look for cross-layer inconsistencies
- Detect non-human behavior patterns

**Full bypass typically requires:**
1. Engine-level mods (Camoufox-style C++ Firefox forks)
2. Human-like behavior emulation
3. Residential/mobile proxies + session persistence
4. Real-time challenge solvers

---

## Implementation Priority for Defender

### Wave 2 (In Progress)
- [ ] Canvas fingerprint consistency
- [ ] Property descriptor tampering detection
- [ ] AudioContext fingerprint

### Wave 3
- [ ] Execution context / stack trace inspection
- [ ] hardwareConcurrency / deviceMemory consistency
- [ ] Proof-of-work micro-challenge

### Wave 4+
- [ ] Mouse movement entropy check (requires interaction)
- [ ] Timing analysis (sub-ms precision detection)
- [ ] Multi-signal consistency correlation
