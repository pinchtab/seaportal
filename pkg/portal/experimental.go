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
// Designed to pass CreepJS and advanced fingerprinting
const stealthScript = `
(function() {
  // ============================================
  // NATIVE FUNCTION WRAPPER UTILITY
  // Makes spoofed functions appear native in toString()
  // ============================================
  const makeNativeFunction = (fn, name) => {
    const wrapped = function() { return fn.apply(this, arguments); };
    wrapped.toString = () => 'function ' + name + '() { [native code] }';
    Object.defineProperty(wrapped, 'name', { value: name });
    return wrapped;
  };

  // ============================================
  // 1. NAVIGATOR PROPERTIES
  // ============================================
  
  // webdriver - must appear native
  Object.defineProperty(navigator, 'webdriver', {
    get: makeNativeFunction(() => false, 'get webdriver'),
    configurable: true
  });
  
  // Hardware concurrency (headless often has 1-2)
  Object.defineProperty(navigator, 'hardwareConcurrency', {
    get: makeNativeFunction(() => 8, 'get hardwareConcurrency'),
    configurable: true
  });
  
  // Device memory (headless often has low values)
  Object.defineProperty(navigator, 'deviceMemory', {
    get: makeNativeFunction(() => 8, 'get deviceMemory'),
    configurable: true
  });
  
  // Languages array
  Object.defineProperty(navigator, 'languages', {
    get: makeNativeFunction(() => Object.freeze(['en-US', 'en']), 'get languages'),
    configurable: true
  });
  
  // Fix HeadlessChrome in UA
  const originalUA = navigator.userAgent;
  if (originalUA.includes('HeadlessChrome')) {
    Object.defineProperty(navigator, 'userAgent', {
      get: makeNativeFunction(() => originalUA.replace('HeadlessChrome', 'Chrome'), 'get userAgent'),
      configurable: true
    });
  }
  
  // ============================================
  // 2. PLUGINS & MIME TYPES (realistic Chrome set)
  // ============================================
  const createPlugin = (name, filename, desc, mimeTypes) => {
    const plugin = { name, filename, description: desc, length: mimeTypes.length };
    mimeTypes.forEach((mt, i) => { plugin[i] = mt; });
    return plugin;
  };
  
  const pdfMime = { type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format' };
  const plugins = [
    createPlugin('Chrome PDF Plugin', 'internal-pdf-viewer', 'Portable Document Format', [pdfMime]),
    createPlugin('Chrome PDF Viewer', 'mhjfbmdgcfjbbpaeojofohoefgiehjai', '', [pdfMime]),
    createPlugin('Native Client', 'internal-nacl-plugin', '', [])
  ];
  plugins.length = 3;
  plugins.item = (i) => plugins[i];
  plugins.namedItem = (name) => plugins.find(p => p.name === name);
  plugins.refresh = () => {};
  
  Object.defineProperty(navigator, 'plugins', {
    get: makeNativeFunction(() => plugins, 'get plugins'),
    configurable: true
  });
  
  // ============================================
  // 3. CDP MARKERS CLEANUP
  // ============================================
  Object.keys(window).filter(k => k.match(/^cdc_|^__webdriver/)).forEach(k => {
    try { delete window[k]; } catch(e) {}
  });
  
  // ============================================
  // 4. CHROME RUNTIME OBJECT
  // ============================================
  if (!window.chrome) window.chrome = {};
  window.chrome.runtime = {
    connect: makeNativeFunction(() => ({}), 'connect'),
    sendMessage: makeNativeFunction(() => {}, 'sendMessage'),
    onMessage: { addListener: makeNativeFunction(() => {}, 'addListener') },
    id: undefined
  };
  window.chrome.csi = makeNativeFunction(() => ({}), 'csi');
  window.chrome.loadTimes = makeNativeFunction(() => ({}), 'loadTimes');
  
  // ============================================
  // 5. PERMISSIONS API
  // ============================================
  const origQuery = navigator.permissions?.query?.bind(navigator.permissions);
  if (origQuery) {
    navigator.permissions.query = makeNativeFunction((params) => {
      if (params.name === 'notifications') {
        return Promise.resolve({ state: 'prompt', onchange: null });
      }
      return origQuery(params);
    }, 'query');
  }
  
  // ============================================
  // 6. WEBGL VENDOR/RENDERER SPOOFING
  // ============================================
  const originalGetParameter = WebGLRenderingContext.prototype.getParameter;
  WebGLRenderingContext.prototype.getParameter = makeNativeFunction(function(param) {
    // UNMASKED_VENDOR_WEBGL
    if (param === 37445) return 'Google Inc. (Apple)';
    // UNMASKED_RENDERER_WEBGL
    if (param === 37446) return 'ANGLE (Apple, Apple M1, OpenGL 4.1)';
    return originalGetParameter.call(this, param);
  }, 'getParameter');
  
  const originalGetParameter2 = WebGL2RenderingContext?.prototype?.getParameter;
  if (originalGetParameter2) {
    WebGL2RenderingContext.prototype.getParameter = makeNativeFunction(function(param) {
      if (param === 37445) return 'Google Inc. (Apple)';
      if (param === 37446) return 'ANGLE (Apple, Apple M1, OpenGL 4.1)';
      return originalGetParameter2.call(this, param);
    }, 'getParameter');
  }
  
  // ============================================
  // 7. AUDIO CONTEXT FINGERPRINT NORMALIZATION
  // ============================================
  const originalCreateOscillator = AudioContext.prototype.createOscillator;
  const originalCreateDynamicsCompressor = AudioContext.prototype.createDynamicsCompressor;
  const originalCreateAnalyser = AudioContext.prototype.createAnalyser;
  
  // Add slight deterministic noise to audio fingerprints
  const audioNoise = 0.0000001;
  
  AudioContext.prototype.createAnalyser = makeNativeFunction(function() {
    const analyser = originalCreateAnalyser.call(this);
    const origGetFloatFrequencyData = analyser.getFloatFrequencyData.bind(analyser);
    analyser.getFloatFrequencyData = makeNativeFunction(function(array) {
      origGetFloatFrequencyData(array);
      for (let i = 0; i < array.length; i++) {
        array[i] += audioNoise * (i % 2 ? 1 : -1);
      }
    }, 'getFloatFrequencyData');
    return analyser;
  }, 'createAnalyser');
  
  // ============================================
  // 8. SCREEN DIMENSIONS (consistent values)
  // ============================================
  Object.defineProperty(screen, 'width', { get: () => 1920 });
  Object.defineProperty(screen, 'height', { get: () => 1080 });
  Object.defineProperty(screen, 'availWidth', { get: () => 1920 });
  Object.defineProperty(screen, 'availHeight', { get: () => 1055 }); // Account for taskbar
  Object.defineProperty(screen, 'colorDepth', { get: () => 24 });
  Object.defineProperty(screen, 'pixelDepth', { get: () => 24 });
  
  // ============================================
  // 9. PERFORMANCE TIMING JITTER
  // ============================================
  const origNow = performance.now.bind(performance);
  let offset = Math.random() * 0.1;
  performance.now = makeNativeFunction(function() {
    // Add small jitter (0.1-0.5ms) to prevent timing analysis
    offset += (Math.random() - 0.5) * 0.1;
    return origNow() + offset;
  }, 'now');
  
  // ============================================
  // 10. BATTERY API (headless often lacks this)
  // ============================================
  if (!navigator.getBattery) {
    navigator.getBattery = makeNativeFunction(() => Promise.resolve({
      charging: true,
      chargingTime: 0,
      dischargingTime: Infinity,
      level: 1.0,
      addEventListener: () => {},
      removeEventListener: () => {}
    }), 'getBattery');
  }
  
  // ============================================
  // 11. CONSOLE IFRAME DETECTION BYPASS
  // ============================================
  const originalFrame = HTMLIFrameElement.prototype;
  Object.defineProperty(originalFrame, 'contentWindow', {
    get: function() {
      return this.contentDocument?.defaultView;
    }
  });
  
  // ============================================
  // 12. WORKER CONTEXT CONSISTENCY
  // Intercept Blob creation to inject spoofs into Worker scripts
  // ============================================
  const workerSpoofScript = ` + "`" + `
    // Spoof navigator properties to match main thread
    Object.defineProperty(navigator, 'languages', {
      get: () => Object.freeze(['en-US', 'en']),
      configurable: true
    });
    Object.defineProperty(navigator, 'hardwareConcurrency', {
      get: () => 8,
      configurable: true
    });
    Object.defineProperty(navigator, 'deviceMemory', {
      get: () => 8,
      configurable: true
    });
    Object.defineProperty(navigator, 'platform', {
      get: () => 'MacIntel',
      configurable: true
    });
  ` + "`" + `;
  
  // Track JavaScript blobs so we can intercept their URLs
  const jsBlobMap = new WeakMap();
  const OriginalBlob = window.Blob;
  
  window.Blob = function(parts, options) {
    const blob = new OriginalBlob(parts, options);
    // Track JavaScript blobs for worker interception
    if (options && options.type && options.type.includes('javascript')) {
      jsBlobMap.set(blob, parts);
    }
    return blob;
  };
  window.Blob.prototype = OriginalBlob.prototype;
  Object.defineProperty(window.Blob, 'name', { value: 'Blob' });
  
  // Intercept createObjectURL to prepend spoofs to JS blobs
  const origCreateObjectURL = URL.createObjectURL.bind(URL);
  URL.createObjectURL = function(obj) {
    // If this is a tracked JavaScript blob, prepend our spoof script
    if (obj instanceof Blob && jsBlobMap.has(obj)) {
      const originalParts = jsBlobMap.get(obj);
      const spoofedBlob = new OriginalBlob([workerSpoofScript + ';\n', ...originalParts], { type: 'application/javascript' });
      return origCreateObjectURL(spoofedBlob);
    }
    return origCreateObjectURL(obj);
  };
  
  // ============================================
  // 13. WEBGL TIMING SPOOFING
  // CreepJS checks MAX_CLIENT_WAIT_TIMEOUT_WEBGL
  // ============================================
  const originalGetExtension = WebGLRenderingContext.prototype.getExtension;
  WebGLRenderingContext.prototype.getExtension = makeNativeFunction(function(name) {
    const ext = originalGetExtension.call(this, name);
    if (name === 'WEBGL_debug_renderer_info' && ext) {
      // Already spoofed in getParameter
      return ext;
    }
    return ext;
  }, 'getExtension');
  
  // Spoof WebGL context attributes
  const originalGetContextAttributes = WebGLRenderingContext.prototype.getContextAttributes;
  WebGLRenderingContext.prototype.getContextAttributes = makeNativeFunction(function() {
    const attrs = originalGetContextAttributes.call(this);
    if (attrs) {
      // Ensure consistent attributes
      attrs.antialias = true;
      attrs.depth = true;
      attrs.stencil = false;
      attrs.alpha = true;
      attrs.premultipliedAlpha = true;
      attrs.preserveDrawingBuffer = false;
      attrs.powerPreference = 'default';
      attrs.failIfMajorPerformanceCaveat = false;
    }
    return attrs;
  }, 'getContextAttributes');
  
  // ============================================
  // 14. ERROR STACK TRACE SANITIZATION
  // Remove automation framework markers from stack traces
  // ============================================
  const originalErrorStack = Object.getOwnPropertyDescriptor(Error.prototype, 'stack');
  if (originalErrorStack && originalErrorStack.get) {
    Object.defineProperty(Error.prototype, 'stack', {
      get: function() {
        const stack = originalErrorStack.get.call(this);
        if (typeof stack === 'string') {
          // Remove CDP/automation markers from stack
          return stack
            .replace(/__puppeteer_evaluation_script__/g, 'anonymous')
            .replace(/__playwright_evaluation_script__/g, 'anonymous')
            .replace(/__selenium_evaluate/g, 'anonymous')
            .replace(/__cdp_binding__/g, 'anonymous')
            .replace(/chrome-extension:\/\/[^\s]+/g, '');
        }
        return stack;
      },
      set: originalErrorStack.set,
      configurable: true
    });
  }
  
  // ============================================
  // 15. SERVICEWORKER CONTEXT CONSISTENCY
  // ============================================
  if (navigator.serviceWorker) {
    const originalRegister = navigator.serviceWorker.register;
    if (originalRegister) {
      navigator.serviceWorker.register = makeNativeFunction(function(scriptURL, options) {
        // Allow ServiceWorker registration but the worker will have native fingerprints
        // which may not match our spoofed main thread - this is hard to fix
        return originalRegister.call(navigator.serviceWorker, scriptURL, options);
      }, 'register');
    }
  }
  
  // ============================================
  // 16. SHARED WORKER SPOOFING
  // ============================================
  if (window.SharedWorker) {
    const OriginalSharedWorker = window.SharedWorker;
    window.SharedWorker = function(scriptURL, options) {
      // For blob URLs, we've already injected via createObjectURL
      // For regular URLs, we can't easily intercept, but most fingerprinters use blobs
      return new OriginalSharedWorker(scriptURL, options);
    };
    window.SharedWorker.prototype = OriginalSharedWorker.prototype;
    Object.defineProperty(window.SharedWorker, 'name', { value: 'SharedWorker' });
  }
  
  // ============================================
  // 14. IFRAME CONTEXT SPOOFING
  // Inject spoofs into dynamically created iframes
  // ============================================
  const iframeSpoofScript = workerSpoofScript + ` + "`" + `;
    // Additional iframe-specific spoofs
    Object.defineProperty(navigator, 'webdriver', {
      get: () => false,
      configurable: true
    });
  ` + "`" + `;
  
  // Intercept iframe creation
  const originalCreateElement = document.createElement.bind(document);
  document.createElement = function(tagName) {
    const element = originalCreateElement(tagName);
    
    if (tagName.toLowerCase() === 'iframe') {
      // Monitor when iframe loads to inject spoofs
      element.addEventListener('load', function() {
        try {
          const iframeWindow = element.contentWindow;
          const iframeDocument = element.contentDocument;
          
          if (iframeWindow && iframeDocument) {
            // Inject spoof script into iframe
            const script = iframeDocument.createElement('script');
            script.textContent = '(' + function(spoofs) {
              try { eval(spoofs); } catch(e) {}
            }.toString() + ')(' + JSON.stringify(iframeSpoofScript) + ')';
            
            if (iframeDocument.head) {
              iframeDocument.head.insertBefore(script, iframeDocument.head.firstChild);
            } else if (iframeDocument.body) {
              iframeDocument.body.insertBefore(script, iframeDocument.body.firstChild);
            }
            
            // Also spoof the iframe's navigator directly
            try {
              Object.defineProperty(iframeWindow.navigator, 'webdriver', {
                get: () => false,
                configurable: true
              });
              Object.defineProperty(iframeWindow.navigator, 'languages', {
                get: () => Object.freeze(['en-US', 'en']),
                configurable: true
              });
              Object.defineProperty(iframeWindow.navigator, 'hardwareConcurrency', {
                get: () => 8,
                configurable: true
              });
              Object.defineProperty(iframeWindow.navigator, 'deviceMemory', {
                get: () => 8,
                configurable: true
              });
            } catch(e) {
              // Cross-origin iframes will throw - that's expected
            }
          }
        } catch(e) {
          // Security errors for cross-origin iframes - expected
        }
      });
    }
    
    return element;
  };
  
  // Also spoof existing iframes on the page
  const spoofExistingIframes = () => {
    document.querySelectorAll('iframe').forEach(iframe => {
      try {
        const iframeWindow = iframe.contentWindow;
        if (iframeWindow && iframeWindow.navigator) {
          Object.defineProperty(iframeWindow.navigator, 'webdriver', {
            get: () => false,
            configurable: true
          });
          Object.defineProperty(iframeWindow.navigator, 'languages', {
            get: () => Object.freeze(['en-US', 'en']),
            configurable: true
          });
          Object.defineProperty(iframeWindow.navigator, 'hardwareConcurrency', {
            get: () => 8,
            configurable: true
          });
        }
      } catch(e) {}
    });
  };
  
  // Run on existing iframes and observe for new ones
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', spoofExistingIframes);
  } else {
    spoofExistingIframes();
  }
  
  // Use MutationObserver to catch dynamically added iframes
  const observer = new MutationObserver(mutations => {
    for (const mutation of mutations) {
      for (const node of mutation.addedNodes) {
        if (node.tagName === 'IFRAME') {
          node.addEventListener('load', () => {
            try {
              const iw = node.contentWindow;
              if (iw && iw.navigator) {
                Object.defineProperty(iw.navigator, 'webdriver', { get: () => false });
                Object.defineProperty(iw.navigator, 'languages', { get: () => Object.freeze(['en-US', 'en']) });
                Object.defineProperty(iw.navigator, 'hardwareConcurrency', { get: () => 8 });
                Object.defineProperty(iw.navigator, 'deviceMemory', { get: () => 8 });
              }
            } catch(e) {}
          });
        }
      }
    }
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  
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
