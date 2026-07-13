package engine

import "testing"

func TestDeriveDecision(t *testing.T) {
	cases := []struct {
		name        string
		result      Result
		profile     PageProfile
		wantDec     BrowserDecision
		wantBrowser bool
	}{
		{
			name:    "static high-confidence SSR extract",
			result:  Result{StatusCode: 200, Length: 3001},
			profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract, Trustworthy: true},
			wantDec: DecisionStaticHighConfidence, wantBrowser: false,
		},
		{
			name:    "plain static extract (FromHTML, no status)",
			result:  Result{StatusCode: 0, Length: 1500},
			profile: PageProfile{Class: PageStatic, Outcome: OutcomeExtract, Trustworthy: true},
			wantDec: DecisionStaticHighConfidence, wantBrowser: false,
		},
		{
			name:    "medium extract, not trustworthy -> static-ok",
			result:  Result{StatusCode: 200, Length: 800},
			profile: PageProfile{Class: PageSSR, Outcome: OutcomeExtract, Trustworthy: false},
			wantDec: DecisionStaticOK, wantBrowser: false,
		},
		{
			name:    "thin clean extract (minimal page) -> static-caution",
			result:  Result{StatusCode: 200, Length: 149},
			profile: PageProfile{Class: PageStatic, Outcome: OutcomeExtract},
			wantDec: DecisionStaticCaution, wantBrowser: false,
		},
		{
			name:    "dynamic warning shell with body -> static-caution",
			result:  Result{StatusCode: 200, Length: 900},
			profile: PageProfile{Class: PageDynamic, Outcome: OutcomeExtractWarning},
			wantDec: DecisionStaticCaution, wantBrowser: false,
		},
		{
			name:    "SPA shell -> browser-needed",
			result:  Result{StatusCode: 200, Length: 200},
			profile: PageProfile{Class: PageSPA, Outcome: OutcomeNeedsBrowser},
			wantDec: DecisionBrowserNeeded, wantBrowser: true,
		},
		{
			name:    "needs-browser dominates high length/quality (auth-wall)",
			result:  Result{StatusCode: 200, Length: 50000, Quality: 95},
			profile: PageProfile{Class: PageSSR, Outcome: OutcomeNeedsBrowser, Reasons: []string{"auth-wall-content"}},
			wantDec: DecisionBrowserNeeded, wantBrowser: true,
		},
		{
			name:    "blocked / bot protection",
			result:  Result{StatusCode: 200, IsBlocked: true},
			profile: PageProfile{Class: PageBlocked, Outcome: OutcomeNeedsBrowser},
			wantDec: DecisionBlocked, wantBrowser: true,
		},
		{
			name:    "401 unauthorized -> auth-required, not browser (ALP-050)",
			result:  Result{StatusCode: 401, IsBlocked: true},
			profile: PageProfile{Class: PageBlocked, Outcome: OutcomeNeedsBrowser, Reasons: []string{"http-401-unauthorized"}},
			wantDec: DecisionAuthRequired, wantBrowser: false,
		},
		{
			name:    "403 forbidden -> blocked, browser (distinct from 401)",
			result:  Result{StatusCode: 403, IsBlocked: true},
			profile: PageProfile{Class: PageBlocked, Outcome: OutcomeNeedsBrowser, Reasons: []string{"http-403-forbidden"}},
			wantDec: DecisionBlocked, wantBrowser: true,
		},
		{
			name:    "transport failure -> unreachable",
			result:  Result{StatusCode: 0, Error: "dial tcp: lookup nope.example: no such host"},
			profile: PageProfile{Outcome: OutcomeFailFast},
			wantDec: DecisionUnreachable, wantBrowser: false,
		},
		{
			name:    "hard 404 -> not-found",
			result:  Result{StatusCode: 404, Length: 300},
			profile: PageProfile{Class: PageStatic, Outcome: OutcomeExtract},
			wantDec: DecisionNotFound, wantBrowser: false,
		},
		{
			name:    "soft 404 -> not-found",
			result:  Result{StatusCode: 200, IsSoft404: true, Length: 200},
			profile: PageProfile{Class: PageStatic, Outcome: OutcomeExtract},
			wantDec: DecisionNotFound, wantBrowser: false,
		},
		{
			name:    "binary content -> unsupported",
			result:  Result{StatusCode: 200, Error: "skipped binary content: image/png"},
			profile: PageProfile{Outcome: OutcomeFailFast},
			wantDec: DecisionUnsupported, wantBrowser: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, browser := deriveDecision(tc.result, tc.profile)
			if dec != tc.wantDec {
				t.Errorf("decision = %q, want %q", dec, tc.wantDec)
			}
			if browser != tc.wantBrowser {
				t.Errorf("browserRecommended = %v, want %v", browser, tc.wantBrowser)
			}
		})
	}
}

func TestClassifyPage_AlwaysSetsDecision(t *testing.T) {
	r := Result{StatusCode: 200, Length: 2000, HeadingCount: 3, ParagraphCount: 5, Confidence: 90}
	p := ClassifyPage(r)
	if p.Decision == "" {
		t.Fatalf("ClassifyPage left Decision empty: %+v", p)
	}
}
