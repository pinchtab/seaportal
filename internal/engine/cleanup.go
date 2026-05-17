package engine

import (
	"regexp"
	"strings"
)

var (
	cdataBareRE   = regexp.MustCompile(`(?s)<!\\?\[CDATA\\?\[(.*?)\\?\]\\?\]>`)
	cdataEntityRE = regexp.MustCompile(`(?s)&lt;!\\?\[CDATA\\?\[(.*?)\\?\]\\?\]&gt;`)
	// regression: wikidata-edit-property-pencil-leak — Wikipedia infobox edit
	// pencils render as image-only links to Wikidata Q*#P* anchors. They're
	// chrome, not content, and appear identically across every language
	// edition (de/es/zh/ar/ru). URL-pattern match so the rule is cross-cutting,
	// not site-specific.
	wikidataEditPropertyRE = regexp.MustCompile(`\[!\[[^\]]*\]\([^)]*\)\]\(https?://(?:www\.)?wikidata\.org/wiki/Q\d+#P\d+(?:\s+"[^"]*")?\)`)
	// regression: wikidata-edit-property-pencil-leak — MediaWiki section-edit
	// anchors (`?action=edit`, `?veaction=edit`) render as text links like
	// `[edit](...)`/`[تعديل](...)` and are pure chrome on every MediaWiki
	// instance, not just Wikipedia.
	mediaWikiEditLinkRE = regexp.MustCompile(`\[[^\]]*\]\(https?://[^)\s]*[?&](?:ve)?action=edit[^)]*\)`)
)

func CleanupMarkdown(md string) string {
	md = cdataBareRE.ReplaceAllString(md, "$1")
	md = cdataEntityRE.ReplaceAllString(md, "$1")
	md = wikidataEditPropertyRE.ReplaceAllString(md, "")
	md = mediaWikiEditLinkRE.ReplaceAllString(md, "")

	commentRe := regexp.MustCompile(`<!--[^>]*-->`)
	md = commentRe.ReplaceAllString(md, "")

	emptyLinkListRe := regexp.MustCompile(`(?m)^[\-\*\+]\s*\[\]\([^)]+\)\s*$`)
	md = emptyLinkListRe.ReplaceAllString(md, "")

	emptyLinkRe := regexp.MustCompile(`\[\]\([^)]+\)`)
	md = emptyLinkRe.ReplaceAllString(md, "")

	appCtaRe := regexp.MustCompile(`(?im)^.*(?:scan the qr code|download the.*app|get the app|available on.*app store|available on.*google play).*$\n?`)
	md = appCtaRe.ReplaceAllString(md, "")

	multiBlankRe := regexp.MustCompile(`\n{3,}`)
	md = multiBlankRe.ReplaceAllString(md, "\n\n")

	return strings.TrimSpace(md)
}
