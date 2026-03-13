package portal

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

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

	// Navigate, wait for render, extract
	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.Sleep(opts.WaitFor),
		chromedp.Title(&title),
		chromedp.InnerHTML("body", &html, chromedp.ByQuery),
		chromedp.OuterHTML("html", &outerHTML, chromedp.ByQuery),
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
