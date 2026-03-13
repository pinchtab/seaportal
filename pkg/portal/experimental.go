package portal

import (
	"context"
	"fmt"
	"time"

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
		opts.WaitFor = 2 * time.Second
	}

	result := ExperimentalResult{
		URL:      targetURL,
		Rendered: true,
	}

	// Create headless Chrome context
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		)...,
	)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var title, html, outerHTML string

	// Navigate and wait for DOM to stabilize
	// Strategy: poll DOM length until it stops changing
	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.Sleep(opts.WaitFor),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForDOMStable(ctx, 500*time.Millisecond, 5)
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
		// Force visibility on hidden content (animation placeholders, lazy sections)
		chromedp.ActionFunc(func(ctx context.Context) error {
			var done bool
			return chromedp.Evaluate(forceVisibilityScript, &done).Do(ctx)
		}),
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
