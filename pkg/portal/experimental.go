package portal

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// triggerAnimationEndScript fires animationend events to unlock animation-gated content
const triggerAnimationEndScript = `
(function() {
  // Find all elements with CSS animations and fire animationend
  var animated = document.querySelectorAll('[class*="animation"], [class*="animate"], [style*="animation"]');
  animated.forEach(function(el) {
    var styles = getComputedStyle(el);
    var animName = styles.animationName;
    if (animName && animName !== 'none') {
      el.dispatchEvent(new AnimationEvent('animationend', { animationName: animName, bubbles: true }));
    }
  });
  
  // Also check elements with animation classes we know about
  document.querySelectorAll('.spotlight-container, [class*="reveal"], [class*="fade"]').forEach(function(el) {
    var styles = getComputedStyle(el);
    var animName = styles.animationName;
    if (animName && animName !== 'none') {
      el.dispatchEvent(new AnimationEvent('animationend', { animationName: animName, bubbles: true }));
    }
  });
  
  return true;
})()
`

// simulateMouseMovement uses CDP Input.dispatchMouseEvent for realistic mouse simulation
// that triggers all DOM event listeners (unlike synthetic JS events).
// Must complete within ~400ms to beat detection script's 500ms collection window.
func simulateMouseMovement(ctx context.Context) error {
	// Random seed from current time
	seed := time.Now().UnixNano()

	// Generate ~25-35 mouse move points with curved path and variance
	numPoints := 25 + int(seed%10)

	// Starting position
	x := 200.0 + float64(seed%200)
	y := 250.0 + float64((seed/1000)%150)

	for i := 0; i < numPoints; i++ {
		// Curved path: base movement + sinusoidal deviation + jitter
		baseX := x + float64(i*12)
		baseY := y + float64(i%8*18) - float64(i%3*6)

		// Add natural curve deviation
		curveX := baseX + float64(i%5*8)
		curveY := baseY + float64(i%7*5)

		// Add micro-jitter (hand tremor)
		jitterX := float64((seed+int64(i*7))%5) - 2.0
		jitterY := float64((seed+int64(i*11))%5) - 2.0

		finalX := curveX + jitterX
		finalY := curveY + jitterY

		// Dispatch CDP mouse move event
		input.DispatchMouseEvent(input.MouseMoved, finalX, finalY).Do(ctx)

		// Variable delay: 8-16ms per point (~300ms total for 30 points)
		delay := time.Duration(8+(seed+int64(i))%8) * time.Millisecond
		time.Sleep(delay)
	}

	return nil
}

// mouseEntropyScript simulates human-like mouse movements to pass behavioral detection
const mouseEntropyScript = `
(function() {
  // Bézier curve helper for smooth, curved paths
  function bezierPoint(t, p0, p1, p2, p3) {
    const u = 1 - t;
    return u*u*u*p0 + 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t*p3;
  }
  
  // Generate human-like mouse path using cubic Bézier curves
  function generatePath(startX, startY, endX, endY, steps) {
    const points = [];
    
    // Control points with randomness for natural curves
    const cp1x = startX + (endX - startX) * 0.25 + (Math.random() - 0.5) * 100;
    const cp1y = startY + (endY - startY) * 0.1 + (Math.random() - 0.5) * 80;
    const cp2x = startX + (endX - startX) * 0.75 + (Math.random() - 0.5) * 100;
    const cp2y = startY + (endY - startY) * 0.9 + (Math.random() - 0.5) * 80;
    
    for (let i = 0; i <= steps; i++) {
      // Non-linear t for acceleration/deceleration (ease-in-out)
      let t = i / steps;
      t = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
      
      const x = bezierPoint(t, startX, cp1x, cp2x, endX);
      const y = bezierPoint(t, startY, cp1y, cp2y, endY);
      
      // Add micro-jitter to simulate hand tremor
      const jitterX = (Math.random() - 0.5) * 2;
      const jitterY = (Math.random() - 0.5) * 2;
      
      points.push({ x: x + jitterX, y: y + jitterY });
    }
    return points;
  }
  
  // Dispatch synthetic mouse events
  function dispatchMouse(type, x, y) {
    const event = new MouseEvent(type, {
      bubbles: true,
      cancelable: true,
      view: window,
      clientX: x,
      clientY: y,
      screenX: x,
      screenY: y
    });
    document.elementFromPoint(x, y)?.dispatchEvent(event) || document.body.dispatchEvent(event);
  }
  
  // Simulate realistic mouse movement
  async function simulateMovement() {
    const viewW = window.innerWidth;
    const viewH = window.innerHeight;
    
    // Start from random position
    let x = Math.random() * viewW * 0.8 + viewW * 0.1;
    let y = Math.random() * viewH * 0.8 + viewH * 0.1;
    
    // Make 2-3 movements with pauses
    const numMoves = 2 + Math.floor(Math.random() * 2);
    
    for (let m = 0; m < numMoves; m++) {
      // Random target
      const targetX = Math.random() * viewW * 0.8 + viewW * 0.1;
      const targetY = Math.random() * viewH * 0.8 + viewH * 0.1;
      
      // Generate curved path with 15-25 points
      const steps = 15 + Math.floor(Math.random() * 10);
      const path = generatePath(x, y, targetX, targetY, steps);
      
      // Move along path with variable speed
      for (const point of path) {
        dispatchMouse('mousemove', point.x, point.y);
        // Variable delay: 10-30ms per step (mimics human reaction time variance)
        await new Promise(r => setTimeout(r, 10 + Math.random() * 20));
      }
      
      // Occasional hover pause (50-150ms)
      if (Math.random() > 0.5) {
        await new Promise(r => setTimeout(r, 50 + Math.random() * 100));
      }
      
      x = targetX;
      y = targetY;
    }
    
    return true;
  }
  
  // Execute and return promise
  return simulateMovement();
})()
`

// stealthScript injects anti-detection bypasses before page loads
const stealthScript = `
(function() {
  // 1. Override navigator.webdriver
  Object.defineProperty(navigator, 'webdriver', {
    get: () => false,
    configurable: true
  });
  
  // 2. Delete CDP markers
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;
  
  // 3. Mock plugins array
  Object.defineProperty(navigator, 'plugins', {
    get: () => {
      const plugins = [
        { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer', description: 'Portable Document Format' },
        { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '' },
        { name: 'Native Client', filename: 'internal-nacl-plugin', description: '' }
      ];
      plugins.length = 3;
      return plugins;
    },
    configurable: true
  });
  
  // 4. Mock languages
  Object.defineProperty(navigator, 'languages', {
    get: () => ['en-US', 'en'],
    configurable: true
  });
  
  // 5. Fix user agent if HeadlessChrome present
  if (navigator.userAgent.includes('HeadlessChrome')) {
    Object.defineProperty(navigator, 'userAgent', {
      get: () => navigator.userAgent.replace('HeadlessChrome', 'Chrome'),
      configurable: true
    });
  }
  
  // 6. Mock permissions
  const originalQuery = navigator.permissions?.query;
  if (originalQuery) {
    navigator.permissions.query = (parameters) => {
      if (parameters.name === 'notifications') {
        return Promise.resolve({ state: 'prompt', onchange: null });
      }
      return originalQuery.call(navigator.permissions, parameters);
    };
  }
  
  // 7. Mock chrome.runtime
  if (!window.chrome) window.chrome = {};
  if (!window.chrome.runtime) {
    window.chrome.runtime = {
      connect: () => {},
      sendMessage: () => {},
      onMessage: { addListener: () => {} }
    };
  }
  
  return true;
})()
`

// forceVisibilityScript reveals hidden content before extraction.
// This handles CSS-animated content, lazy placeholders, and display:none sections.
const forceVisibilityScript = `
(function() {
  // Inject CSS to force all elements visible
  var style = document.createElement('style');
  style.textContent = '* { visibility: visible !important; opacity: 1 !important; height: auto !important; overflow: visible !important; }';
  document.head.appendChild(style);
  
  // Remove animation-related classes that might hide content
  document.querySelectorAll('[class*="pending"], [class*="loading"], [class*="hidden"]').forEach(function(el) {
    el.style.visibility = 'visible';
    el.style.opacity = '1';
    el.style.height = 'auto';
    el.style.overflow = 'visible';
  });
  
  return true;
})()
`

// waitForAnimationsScript waits for CSS animations to complete (sync version)
const waitForAnimationsScript = `
(function() {
  var animations = document.getAnimations ? document.getAnimations() : [];
  // Just check if there are running animations - we'll wait via Sleep
  return animations.length;
})()
`

// flattenShadowDOMScript is JS that serializes the full DOM including Shadow DOM content.
const flattenShadowDOMScript = `
(function() {
  function serialize(node) {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    
    var tag = node.tagName.toLowerCase();
    var attrs = '';
    for (var i = 0; i < node.attributes.length; i++) {
      var a = node.attributes[i];
      attrs += ' ' + a.name + '="' + a.value.replace(/"/g, '&quot;') + '"';
    }
    
    var children = '';
    // If element has shadow root, serialize its content
    if (node.shadowRoot) {
      var shadowChildren = node.shadowRoot.childNodes;
      for (var j = 0; j < shadowChildren.length; j++) {
        children += serialize(shadowChildren[j]);
      }
    }
    // Also serialize light DOM children
    var lightChildren = node.childNodes;
    for (var k = 0; k < lightChildren.length; k++) {
      children += serialize(lightChildren[k]);
    }
    
    return '<' + tag + attrs + '>' + children + '</' + tag + '>';
  }
  return serialize(document.documentElement);
})()
`

// waitForDOMStable polls the DOM length and waits until it stops changing.
// It checks every `interval` for up to `maxChecks` rounds.
func waitForDOMStable(ctx context.Context, interval time.Duration, maxChecks int) error {
	var prevLen int
	stableCount := 0

	for i := 0; i < maxChecks; i++ {
		var curLen int
		if err := chromedp.Evaluate(`document.body.innerHTML.length`, &curLen).Do(ctx); err != nil {
			return err
		}

		if curLen == prevLen && curLen > 0 {
			stableCount++
			if stableCount >= 2 {
				return nil // DOM hasn't changed for 2 consecutive checks
			}
		} else {
			stableCount = 0
		}

		prevLen = curLen
		time.Sleep(interval)
	}

	return nil // max checks reached, proceed anyway
}

// ExperimentalOptions configures browser-based extraction
type ExperimentalOptions struct {
	Timeout  time.Duration // Page load timeout (default 30s)
	WaitFor  time.Duration // Wait after load before extracting (default 2s)
	Snapshot bool          // Also produce accessibility snapshot
	Stealth  bool          // Enable stealth mode to bypass bot detection
}

// ExperimentalResult holds the browser-rendered extraction
type ExperimentalResult struct {
	URL       string        `json:"url"`
	Title     string        `json:"title"`
	HTML      string        `json:"html,omitempty"`
	Content   string        `json:"content"`
	Snapshot  *SnapshotNode `json:"snapshot,omitempty"`
	TimeMs    int64         `json:"timeMs"`
	Rendered  bool          `json:"rendered"`
	Error     string        `json:"error,omitempty"`
}

// FromURLExperimental renders a page in headless Chrome and extracts content
func FromURLExperimental(targetURL string, opts ExperimentalOptions) ExperimentalResult {
	start := time.Now()

	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.WaitFor == 0 {
		opts.WaitFor = 5 * time.Second // Extended to catch late-loading content (blob URLs, etc.)
	}

	result := ExperimentalResult{
		URL:      targetURL,
		Rendered: true,
	}

	// Build Chrome flags - stealth mode uses new headless with WebGL support
	var chromeFlags []chromedp.ExecAllocatorOption
	if opts.Stealth {
		chromeFlags = []chromedp.ExecAllocatorOption{
			chromedp.Flag("headless", "new"), // New headless mode has better WebGL support
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			// Stealth-specific flags
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("disable-features", "TranslateUI"),
			chromedp.Flag("disable-infobars", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("disable-extensions", true),
			// WebGL support in headless
			chromedp.Flag("use-gl", "angle"),
			chromedp.Flag("use-angle", "metal"), // Use Metal on macOS for GPU
			chromedp.Flag("enable-webgl", true),
			chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
		}
	} else {
		chromeFlags = []chromedp.ExecAllocatorOption{
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		}
	}

	// Create headless Chrome context
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromeFlags...)...,
	)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var title, html, outerHTML string

	// Inject stealth script before navigation if enabled
	if opts.Stealth {
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Inject stealth script that runs on every new document
				_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
				return err
			}),
		)
		if err != nil {
			// Non-fatal: continue without stealth injection
			fmt.Printf("Warning: stealth injection failed: %v\n", err)
		}
	}

	// Navigate and wait for DOM to stabilize
	// Strategy: poll DOM length until it stops changing
	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		// Quick start: 50ms lets page scripts set up event listeners
		chromedp.Sleep(50*time.Millisecond),
		// Run CDP-based mouse simulation within detection's 500ms window
		chromedp.ActionFunc(func(ctx context.Context) error {
			if opts.Stealth {
				return simulateMouseMovement(ctx)
			}
			return nil
		}),
		chromedp.Sleep(opts.WaitFor),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForDOMStable(ctx, 500*time.Millisecond, 8) // Extended for late-loading content
		}),
		// Wait for CSS animations to complete
		chromedp.ActionFunc(func(ctx context.Context) error {
			var animCount int
			if err := chromedp.Evaluate(waitForAnimationsScript, &animCount).Do(ctx); err != nil {
				return nil // ignore errors, proceed anyway
			}
			if animCount > 0 {
				time.Sleep(1 * time.Second) // give animations time to complete
			}
			return nil
		}),
		// Trigger animationend events to unlock animation-gated content
		chromedp.ActionFunc(func(ctx context.Context) error {
			var done bool
			chromedp.Evaluate(triggerAnimationEndScript, &done).Do(ctx)
			time.Sleep(200 * time.Millisecond) // let event handlers run
			return nil
		}),
		// NOTE: forceVisibilityScript removed — its CSS overrides (* { height: auto !important })
		// broke readability parser, causing 0-byte extractions. The triggerAnimationEndScript
		// above handles animation-gated content without side effects.
		chromedp.Title(&title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Extract HTML with Shadow DOM content flattened into the light DOM
			return chromedp.Evaluate(flattenShadowDOMScript, &outerHTML).Do(ctx)
		}),
		chromedp.InnerHTML("body", &html, chromedp.ByQuery),
	)
	if err != nil {
		result.Error = fmt.Sprintf("browser render failed: %v", err)
		result.TimeMs = time.Since(start).Milliseconds()
		return result
	}

	result.Title = title
	result.HTML = outerHTML

	// Extract content using readability on the rendered HTML
	content, err := ExtractFromHTML(outerHTML, targetURL)
	if err != nil {
		// Fallback: use raw body text
		result.Content = html
	} else {
		result.Content = content
	}

	// Build snapshot if requested
	if opts.Snapshot {
		snap, err := BuildSnapshot(outerHTML)
		if err == nil {
			result.Snapshot = snap
		}
	}

	result.TimeMs = time.Since(start).Milliseconds()
	return result
}
