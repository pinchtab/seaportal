package engine

import (
	"net/url"
	"strings"
)

// authWallURLPathHints are URL path substrings that strongly suggest the page
// IS a login/signup wall (the URL is honest about its purpose).
var authWallURLPathHints = []string{"/login", "/sign-in", "/signin", "/sign-up", "/signup", "/auth"}

// authWallQueryHints are query keys that indicate a post-login redirect
// target — pages reached via these hints are typically auth walls.
var authWallQueryHints = []string{"next", "return_to", "returnto", "redirect", "redirect_to", "continue"}

// authWallCTAs is the CTA vocabulary counted for signal 3. Each distinct
// match counts once. Includes both spaced and unspaced forms ("sign up" and
// "signup") because anchor hrefs like /signup and button labels both surface
// in the readability-extracted markdown body.
var authWallCTAs = []string{
	"log in", "sign in", "sign up", "join now", "continue with",
	"create account", "create an account", "register", "welcome back",
	"signup", "signin", "login",
}

// authWallLinkPaths are URL-path fragments that, when they appear inside
// markdown link targets (e.g. `](…/signup)`), indicate the
// chrome of the page is dominated by auth/onboarding links. This is a purely
// generic content signal — no host knowledge is involved.
var authWallLinkPaths = []string{"/signup", "/sign-up", "/signin", "/sign-in", "/login", "/register", "/join"}

// detectAuthWallByContent decides whether a successfully-extracted page is
// actually a logged-out auth wall, based purely on content + URL signals
// (no host list, no per-property allow/deny rules). Trigger requires QUORUM:
// at least 2 independent signals must fire.
//
// Reason returned: "auth-wall-content".
//
// Backward-compat note: existing JSONL reports + capability suite step 6.7
// previously asserted reason "auth-wall-marketing". The capability suite has
// been updated to accept EITHER string during the transition; downstream
// consumers should migrate to "auth-wall-content".
//
// Signals:
//  1. Structural login form (hasLoginFormMarkers). Counts as 2 signals when
//     the surrounding content has no article structure (ParagraphCount<3
//     AND Length<1500) — a form-shaped page with no prose is almost
//     certainly the login wall itself.
//  2. URL hints — path or query indicates login/redirect.
//  3. CTA density without article structure: >=3 distinct CTAs AND
//     ParagraphCount<3.
//  4. Content-absence + CTA dominance: Length<1500 AND ctaHits/wordCount>0.05.
//  5. Auth-link dominance: >=2 occurrences of `/signup`, `/login`, `/register`
//     etc. inside markdown link targets in the body. Catches landing pages
//     whose chrome links to signup/login from many nav/footer positions even
//     when the visible prose looks article-shaped.
func detectAuthWallByContent(result Result) (bool, string) {
	parsedURL, err := url.Parse(result.URL)
	if err != nil {
		return false, ""
	}
	path := strings.ToLower(parsedURL.Path)
	query := parsedURL.Query()
	content := strings.ToLower(result.Content)

	signals := 0

	// Pre-count CTA hits — used by signals 1 (form boost), 3, and 4.
	distinctCTAs := 0
	totalCTAHits := 0
	for _, cta := range authWallCTAs {
		if c := strings.Count(content, cta); c > 0 {
			distinctCTAs++
			totalCTAHits += c
		}
	}

	// Signal 1 — structural login form. The "email-field + password" pair
	// almost never appears in real article content (docs/wikis/READMEs talk
	// ABOUT passwords but don't render a credential prompt). When this pair
	// surfaces in the extracted body it is a strong signal on its own; we
	// reinforce it to 2 signals when paired with any other auth-wall hint:
	//   - no article structure (ParagraphCount<3 AND Length<1500), or
	//   - at least 2 distinct CTAs from the vocabulary (login-page prose).
	if hasLoginFormMarkers(content) {
		signals++
		if (result.ParagraphCount < 3 && result.Length < 1500) || distinctCTAs >= 2 {
			signals++
		}
	}

	// Signal 2 — URL hints (path or query)
	urlHint := false
	for _, hint := range authWallURLPathHints {
		if strings.Contains(path, hint) {
			urlHint = true
			break
		}
	}
	if !urlHint {
		for _, q := range authWallQueryHints {
			if query.Get(q) != "" {
				urlHint = true
				break
			}
		}
	}
	if urlHint {
		signals++
	}

	// Signal 3 — CTA density on a non-article page.
	if distinctCTAs >= 3 && result.ParagraphCount < 3 {
		signals++
	}

	// Signal 4 — content-absence + CTA dominance
	if result.Length < 1500 && result.Length > 0 {
		words := len(strings.Fields(content))
		if words > 0 {
			ratio := float64(totalCTAHits) / float64(words)
			if ratio > 0.05 {
				signals++
			}
		}
	}

	// Signal 5 — auth-link dominance. Count markdown link targets that point
	// to login/signup/register endpoints. Two or more such links suggest the
	// page chrome is built around onboarding rather than article content.
	authLinkHits := 0
	for _, p := range authWallLinkPaths {
		authLinkHits += strings.Count(content, p+")")     // `](…/signup)` ending
		authLinkHits += strings.Count(content, p+"?")     // `](…/signup?next=…)`
		authLinkHits += strings.Count(content, p+"/)")    // `](…/signup/)`
		authLinkHits += strings.Count(content, p+"/?")    // `](…/signup/?…)`
		authLinkHits += strings.Count(content, "("+p+")") // bare `(/login)` form (HN style)
	}
	if authLinkHits >= 2 {
		signals++
	}

	if signals < 2 {
		return false, ""
	}
	return true, "auth-wall-content"
}
