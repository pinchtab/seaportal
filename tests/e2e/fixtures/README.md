# Fixture Levels

Progressive difficulty levels for testing SeaPortal extraction and stealth capabilities.

## Extraction Levels (`extraction/`)

Test content extraction from increasingly complex JavaScript patterns.

| Level | File | Features | Status |
|-------|------|----------|--------|
| 1 | `level-01-basic.html` | React rendering, setTimeout async, computed values | ✅ Pass |
| 2 | `level-02-shadow.html` | + Shadow DOM, IntersectionObserver | ✅ Pass |
| 3 | `level-03-animations.html` | + RAF, Web Workers, CSS animations | ✅ Pass |
| 4 | `level-04-advanced.html` | + IndexedDB, Blob URLs, requestIdleCallback, postMessage, dynamic import | ✅ Pass |
| 5 | `level-05-frontier.html` | + Service Workers, ResizeObserver | ❌ Gap |

## Detection Levels (`detection/`)

Test bot detection evasion with increasing sophistication.

| Level | File | Checks | Status |
|-------|------|--------|--------|
| 1 | `level-01-basic.html` | webdriver, plugins, languages, UA keywords, screen | ✅ Pass |
| 2 | `level-02-fingerprint.html` | + canvas, WebGL, property descriptors | ⚠️ Partial (WebGL) |
| 3 | `level-03-behavioral.html` | + mouse movement, click patterns, timing | ❌ Gap |

## Usage

Test specific level:
```bash
seaportal --experimental http://localhost:8765/extraction/level-02-shadow.html
seaportal --escape-detection http://localhost:8765/detection/level-01-basic.html
```

Run all levels:
```bash
for f in extraction/level-*.html; do
  echo "Testing $f..."
  seaportal --experimental http://localhost:8765/$f
done
```

## Legacy Fixtures

- `react-app.html` - Evolving mega-fixture (all features combined)
- `bot-detection.html` - Original detection fixture (all checks)

These are kept for backwards compatibility and continuous iteration.
