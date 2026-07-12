package engine

import "testing"

// TestClassifyPageAudit is a standing regression guard over the page classifier
// and coarse content-type inference. It exercises a matrix of representative
// page shapes and pins the observable outputs a caller (e.g. PinchTab) routes on
// — pageClass, isSpa, outcome, decision, browserRecommended, and the coarse
// contentType — so a future classifier change that silently flips a field turns
// a row red instead of surfacing weeks later in a production scrape.
//
// HTML rows run the byte/HTML literal through the real extraction pipeline
// (FromHTMLWithOptions) so Confidence/Length/heading+paragraph counts and SPA
// signals are computed, not hand-set; transport rows (binary/PDF) seed the
// Result the fetch path would produce, because the HTTP Content-Type — not any
// HTML — is the classifier's real input there.
//
// The expected column encodes the corrected behaviour established by ALP-038
// (binary content-type → unsupported), ALP-039 (JS-shell content → needs a
// browser), and ALP-041 (structural content-type fallback). Adding a new page
// shape is one row; a regression is a red row. No network, no new dependency —
// it runs in the normal `go test ./...` path.
type classifyAuditCase struct {
	name string

	// HTML fixture: when non-empty, run through the real extractor at url.
	url  string
	html string

	// Transport fixture: when html is empty, seed stands in for a fetch the HTML
	// pipeline never runs — the ResponseContentType/Error carry the real signal.
	seed *Result

	// Observable classifier outputs. A field flip fails the row.
	wantClass      PageClass
	wantIsSPA      bool
	wantOutcome    ExtractionOutcome
	wantDecision   BrowserDecision
	wantBrowserRec bool
	// Coarse classifyContentType output; asserted only for HTML rows ("" skips).
	wantContentType string
}

func TestClassifyPageAudit(t *testing.T) {
	cases := []classifyAuditCase{
		{
			// Static article with JSON-LD @type=Article and og:type=article.
			name: "static-article-jsonld-og",
			url:  "https://example.com/blog/sourdough",
			html: `<!doctype html><html><head>
<meta property="og:title" content="How Sourdough Rises">
<meta property="og:type" content="article">
<title>How Sourdough Rises</title>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"Article","headline":"How Sourdough Rises","author":{"@type":"Person","name":"Jane Doe"},"datePublished":"2026-01-04"}</script>
</head><body>
<article>
<h1>How Sourdough Rises</h1>
<p>Sourdough gets its lift from a living culture of wild yeast and lactic acid bacteria. As the yeast digests the sugars in the flour it releases carbon dioxide, and the gas is trapped by the elastic gluten network woven through the dough.</p>
<p>The starter is a simple mixture of flour and water kept at room temperature and refreshed every day. Over a week the population of microbes stabilises into a tangy, bubbling colony that a baker can keep alive for years with regular feeding.</p>
<p>During the long bulk ferment the baker folds the dough every half hour. Each fold stretches the gluten strands and redistributes the warmth and the food, building the structure that will hold the airy crumb once the loaf is baked.</p>
<p>A final proof in a cool place lets the flavour deepen while the crumb sets. When the loaf finally meets the hot oven the trapped gas expands in a burst known as oven spring, and the crust caramelises into a deep, glossy shell.</p>
</article>
</body></html>`,
			wantClass:       PageStatic,
			wantIsSPA:       false,
			wantOutcome:     OutcomeExtract,
			wantDecision:    DecisionStaticOK,
			wantBrowserRec:  false,
			wantContentType: "article",
		},
		{
			// Metadata-poor docs page (MDN-shaped): no JSON-LD, no og:type. The
			// coarse type comes from the ALP-041 structural fallback (the /docs/
			// URL segment), not from metadata — a regression there collapses it
			// back to "page"/"unknown".
			name: "metadata-poor-docs-mdn",
			url:  "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/map",
			html: `<!doctype html><html><head>
<title>Array.prototype.map() - JavaScript | MDN</title>
<meta name="description" content="The map() method creates a new array.">
</head><body>
<main><article>
<h1>Array.prototype.map()</h1>
<p>The <code>map()</code> method of Array instances creates a new array populated with the results of calling a provided function on every element in the calling array.</p>
<p>It does not mutate the array on which it is called, although the callback function may do so. The range of elements processed is set before the first call to the callback.</p>
<p>Elements appended to the array after the call to map begins will not be visited by the callback. If existing elements are changed, their value passed to the callback will be the value at the time map visits them.</p>
<p>The map method is a copying method. It does not alter this. However, the function provided as the callback can mutate the array.</p>
</article></main></body></html>`,
			wantClass:       PageStatic,
			wantIsSPA:       false,
			wantOutcome:     OutcomeExtract,
			wantDecision:    DecisionStaticOK,
			wantBrowserRec:  false,
			wantContentType: "article",
		},
		{
			// SPA shell that advertises a root element and a <noscript> warning
			// (excalidraw-shaped): DetectSPA fires on ≥2 raw-HTML markers, so the
			// page is spa/needs-browser with nothing to extract.
			name: "spa-noscript-excalidraw",
			url:  "https://excalidraw.com/",
			html: `<!doctype html><html><head><title>Excalidraw</title></head><body>
<div id="root"></div>
<noscript>You need to enable JavaScript to run this app.</noscript>
</body></html>`,
			wantClass:      PageSPA,
			wantIsSPA:      true,
			wantOutcome:    OutcomeNeedsBrowser,
			wantDecision:   DecisionBrowserNeeded,
			wantBrowserRec: true,
			// T07: with classification reading the extraction Result instead of
			// raw HTML, an empty SPA shell (nothing extractable) is honestly
			// "unknown" rather than "page".
			wantContentType: "unknown",
		},
		{
			// JS-shell whose "enable JavaScript" warning sits in regular DOM, not
			// <noscript> (diagrams.net-shaped): DetectSPA sees only one marker so
			// isSpa stays false, and the content-side isJSShellContent check (ALP-039)
			// is what escalates it to spa/needs-browser.
			name: "js-shell-no-noscript-diagrams",
			url:  "https://app.diagrams.net/",
			html: `<!doctype html><html><head><title>Flowchart Maker &amp; Online Diagram Software</title></head><body>
<div class="geDiagramContainer">
<p>draw.io is free online diagram software for making flowcharts, process diagrams, org charts, UML, ER and network diagrams.</p>
<p>Loading... Please ensure JavaScript is enabled.</p>
</div>
</body></html>`,
			wantClass:       PageSPA,
			wantIsSPA:       false,
			wantOutcome:     OutcomeNeedsBrowser,
			wantDecision:    DecisionBrowserNeeded,
			wantBrowserRec:  true,
			wantContentType: "page",
		},
		{
			// Binary image response: ALP-038 skips it before the HTML pipeline and
			// preserves the Content-Type, so the decision is unsupported and no
			// browser is spent.
			name: "binary-image-png",
			seed: &Result{
				TransportInfo: TransportInfo{ResponseContentType: "image/png"},
				Error:         "skipped binary content: image/png",
				StatusCode:    200,
			},
			wantClass:      PageDynamic,
			wantIsSPA:      false,
			wantOutcome:    OutcomeFailFast,
			wantDecision:   DecisionUnsupported,
			wantBrowserRec: false,
		},
		{
			// Opaque octet-stream download: same unsupported routing as any other
			// binary content-type (ALP-038).
			name: "binary-octet-stream",
			seed: &Result{
				TransportInfo: TransportInfo{ResponseContentType: "application/octet-stream"},
				Error:         "skipped binary content: application/octet-stream",
				StatusCode:    200,
			},
			wantClass:      PageDynamic,
			wantIsSPA:      false,
			wantOutcome:    OutcomeFailFast,
			wantDecision:   DecisionUnsupported,
			wantBrowserRec: false,
		},
		{
			// PDF response: application/pdf is deliberately NOT binary — it flows
			// through text extraction, so a well-extracted PDF classifies as an
			// extractable static/ssr document, never unsupported.
			name: "pdf-extractable",
			seed: &Result{
				TransportInfo:  TransportInfo{ResponseContentType: "application/pdf"},
				StatusCode:     200,
				Content:        "Quarterly Report. Revenue grew twelve percent year over year across all regions.",
				Length:         1200,
				Confidence:     85,
				HeadingCount:   2,
				ParagraphCount: 3,
			},
			wantClass:      PageSSR,
			wantIsSPA:      false,
			wantOutcome:    OutcomeExtract,
			wantDecision:   DecisionStaticHighConfidence,
			wantBrowserRec: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r Result
			if tc.html != "" {
				r = FromHTMLWithOptions(tc.html, tc.url, Options{})
			} else {
				r = *tc.seed
			}
			profile := ClassifyPage(r)

			if profile.Class != tc.wantClass {
				t.Errorf("pageClass = %q, want %q (reasons=%v)", profile.Class, tc.wantClass, profile.Reasons)
			}
			if r.IsSPA != tc.wantIsSPA {
				t.Errorf("isSpa = %v, want %v (spaSignals=%v)", r.IsSPA, tc.wantIsSPA, r.SPASignals)
			}
			if profile.Outcome != tc.wantOutcome {
				t.Errorf("outcome = %q, want %q", profile.Outcome, tc.wantOutcome)
			}
			if profile.Decision != tc.wantDecision {
				t.Errorf("decision = %q, want %q", profile.Decision, tc.wantDecision)
			}
			if profile.BrowserRecommended != tc.wantBrowserRec {
				t.Errorf("browserRecommended = %v, want %v", profile.BrowserRecommended, tc.wantBrowserRec)
			}
			if tc.wantContentType != "" {
				// T07: classifyContentType consumes the extraction Result (the
				// converged scrape path retains no raw HTML).
				got := classifyContentType(r, tc.url)
				if got != tc.wantContentType {
					t.Errorf("contentType = %q, want %q", got, tc.wantContentType)
				}
			}
		})
	}
}
