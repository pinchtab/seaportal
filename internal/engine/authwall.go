package engine

import (
	"net/url"
	"regexp"
	"strings"
)

var authWallTitlePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(log ?in|sign ?in|join)\b[^.]{0,20}\bor\b[^.]{0,20}\b(sign ?up|register|log ?in|join)\b`),
	regexp.MustCompile(`(?i)^\s*(log ?in|sign ?in|sign ?up)\b`),
}

var authWallURLPathHints = []string{"/login", "/sign-in", "/signin", "/sign-up", "/signup", "/auth"}

var authWallQueryHints = []string{"next", "return_to", "returnto", "redirect", "redirect_to", "continue"}

var authWallCTAs = []string{
	"log in", "sign in", "sign up", "join now", "continue with",
	"create account", "create an account", "register", "welcome back",
	"signup", "signin", "login",
}

var authWallLinkPaths = []string{"/signup", "/sign-up", "/signin", "/sign-in", "/login", "/register", "/join"}

func detectAuthWallByContent(result Result) (bool, string) {
	parsedURL, err := url.Parse(result.URL)
	if err != nil {
		return false, ""
	}
	path := strings.ToLower(parsedURL.Path)
	query := parsedURL.Query()
	content := strings.ToLower(result.Content)

	signals := 0

	distinctCTAs := 0
	totalCTAHits := 0
	for _, cta := range authWallCTAs {
		if c := strings.Count(content, cta); c > 0 {
			distinctCTAs++
			totalCTAHits += c
		}
	}

	if hasLoginFormMarkers(content) {
		signals++
		if (result.ParagraphCount < 3 && result.Length < 1500) || distinctCTAs >= 2 {
			signals++
		}
	}

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

	if distinctCTAs >= 3 && result.ParagraphCount < 3 {
		signals++
	}

	if result.Length < 1500 && result.Length > 0 {
		words := len(strings.Fields(content))
		if words > 0 {
			ratio := float64(totalCTAHits) / float64(words)
			if ratio > 0.05 {
				signals++
			}
		}
	}

	authLinkHits := 0
	for _, p := range authWallLinkPaths {
		authLinkHits += strings.Count(content, p+")")
		authLinkHits += strings.Count(content, p+"?")
		authLinkHits += strings.Count(content, p+"/)")
		authLinkHits += strings.Count(content, p+"/?")
		authLinkHits += strings.Count(content, "("+p+")")
	}
	if authLinkHits >= 2 {
		signals++
	}

	for _, re := range authWallTitlePatterns {
		if re.MatchString(result.Title) {
			signals++
			break
		}
	}

	if signals < 2 {
		return false, ""
	}
	return true, "auth-wall-content"
}
