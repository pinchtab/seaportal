package engine

// metadata.go — Unified <meta> tag extractor. Walks every <meta> tag once,
// extracts the attributes we care about (name/property/http-equiv/itemprop +
// content), then resolves a priority chain per Result field. Companion to
// ldjson_metadata.go: JSON-LD runs first with unconditional overwrite;
// applyMetadata fills the remaining gaps so the long tail of conventions
// (OpenGraph, article:*, Dublin Core, classic <meta name>, citation_author)
// still reaches the Result.

import (
	"html"
	"regexp"
	"strings"
)

// Metadata holds the resolved per-field winners from a single HTML pass.
type Metadata struct {
	Author        string
	PublishedDate string
	Language      string
	Section       string
	Description   string
	ImageURL      string
	OGType        string
	Keywords      string
}

var (
	metaTagRE  = regexp.MustCompile(`(?is)<meta\b[^>]*?/?>`)
	metaAttrRE = regexp.MustCompile(`(?is)\b(name|property|http-equiv|itemprop|content)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	htmlLangRE = regexp.MustCompile(`(?is)<html\b[^>]*?\blang\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// ExtractMetadata walks every <meta> tag once and resolves the per-field
// priority chains documented in the user story.
func ExtractMetadata(rawHTML string) Metadata {
	var m Metadata
	if rawHTML == "" {
		return m
	}

	// Per-source slot collectors. Multi-value (authors) accumulate; the rest
	// keep the first non-empty value to mirror "first wins within a tier".
	var (
		ogTitle, ogDesc, ogImage, ogLocale, ogType string
		twitterImage                               string
		artAuthors                                 []string
		artPublished, artSection                   string
		nameAuthor, nameDesc, nameKeywords         string
		dcCreator, dcDate, dcLanguage, dcSubject   string
		dcDescription                              string
		httpEquivLang                              string
		itemPropDate                               string
		citationAuthors                            []string
	)
	seenCitation := map[string]struct{}{}
	seenArt := map[string]struct{}{}

	tags := metaTagRE.FindAllString(rawHTML, -1)
	for _, tag := range tags {
		var name, property, httpEquiv, itemProp, content string
		hasContent := false
		for _, am := range metaAttrRE.FindAllStringSubmatch(tag, -1) {
			key := strings.ToLower(am[1])
			val := am[2]
			if val == "" {
				val = am[3]
			}
			if val == "" {
				val = am[4]
			}
			switch key {
			case "name":
				name = strings.ToLower(strings.TrimSpace(val))
			case "property":
				property = strings.ToLower(strings.TrimSpace(val))
			case "http-equiv":
				httpEquiv = strings.ToLower(strings.TrimSpace(val))
			case "itemprop":
				itemProp = strings.ToLower(strings.TrimSpace(val))
			case "content":
				content = strings.TrimSpace(html.UnescapeString(val))
				hasContent = true
			}
		}
		if !hasContent || content == "" {
			continue
		}

		switch property {
		case "og:title":
			if ogTitle == "" {
				ogTitle = content
			}
		case "og:description":
			if ogDesc == "" {
				ogDesc = content
			}
		case "og:image":
			if ogImage == "" {
				ogImage = content
			}
		case "og:locale":
			if ogLocale == "" {
				ogLocale = content
			}
		case "og:type":
			if ogType == "" {
				ogType = content
			}
		case "article:author":
			if _, ok := seenArt[content]; !ok {
				seenArt[content] = struct{}{}
				artAuthors = append(artAuthors, content)
			}
		case "article:published_time":
			if artPublished == "" {
				artPublished = content
			}
		case "article:section":
			if artSection == "" {
				artSection = content
			}
		}

		switch name {
		case "author":
			if nameAuthor == "" {
				nameAuthor = content
			}
		case "description":
			if nameDesc == "" {
				nameDesc = content
			}
		case "keywords":
			if nameKeywords == "" {
				nameKeywords = content
			}
		case "dc.creator":
			if dcCreator == "" {
				dcCreator = content
			}
		case "dc.date":
			if dcDate == "" {
				dcDate = content
			}
		case "dc.language":
			if dcLanguage == "" {
				dcLanguage = content
			}
		case "dc.subject":
			if dcSubject == "" {
				dcSubject = content
			}
		case "dc.description":
			if dcDescription == "" {
				dcDescription = content
			}
		case "citation_author":
			if _, ok := seenCitation[content]; !ok {
				seenCitation[content] = struct{}{}
				citationAuthors = append(citationAuthors, content)
			}
		case "twitter:image":
			if twitterImage == "" {
				twitterImage = content
			}
		}

		if httpEquiv == "content-language" && httpEquivLang == "" {
			httpEquivLang = content
		}
		if itemProp == "datepublished" && itemPropDate == "" {
			itemPropDate = content
		}
	}

	_ = ogTitle // OGTitle not yet on Metadata struct; reserved for future use.

	// Author priority: article:author > name=author > DC.creator > citation_author.
	switch {
	case len(artAuthors) > 0:
		m.Author = strings.Join(artAuthors, "; ")
	case nameAuthor != "":
		m.Author = nameAuthor
	case dcCreator != "":
		m.Author = dcCreator
	case len(citationAuthors) > 0:
		m.Author = strings.Join(citationAuthors, "; ")
	}

	// PublishedDate: article:published_time > DC.date > itemprop=datePublished.
	switch {
	case artPublished != "":
		m.PublishedDate = artPublished
	case dcDate != "":
		m.PublishedDate = dcDate
	case itemPropDate != "":
		m.PublishedDate = itemPropDate
	}

	// Language: og:locale > http-equiv=content-language > DC.language > <html lang>.
	switch {
	case ogLocale != "":
		m.Language = ogLocale
	case httpEquivLang != "":
		m.Language = httpEquivLang
	case dcLanguage != "":
		m.Language = dcLanguage
	default:
		if hm := htmlLangRE.FindStringSubmatch(rawHTML); hm != nil {
			val := hm[1]
			if val == "" {
				val = hm[2]
			}
			if val == "" {
				val = hm[3]
			}
			m.Language = strings.TrimSpace(val)
		}
	}

	// Section: article:section > DC.subject.
	switch {
	case artSection != "":
		m.Section = artSection
	case dcSubject != "":
		m.Section = dcSubject
	}

	// Description: og:description > name=description > DC.description.
	switch {
	case ogDesc != "":
		m.Description = ogDesc
	case nameDesc != "":
		m.Description = nameDesc
	case dcDescription != "":
		m.Description = dcDescription
	}

	// ImageURL: og:image > twitter:image.
	switch {
	case ogImage != "":
		m.ImageURL = ogImage
	case twitterImage != "":
		m.ImageURL = twitterImage
	}

	m.OGType = ogType

	// Keywords: name=keywords > DC.subject.
	switch {
	case nameKeywords != "":
		m.Keywords = nameKeywords
	case dcSubject != "":
		m.Keywords = dcSubject
	}

	return m
}

// applyMetadata fills Result fields ONLY when currently empty. This preserves
// applyLDJSONMetadata's earlier unconditional-overwrite priority — JSON-LD
// values stay; everything else gets backfilled from the unified pass.
//
// For Author, we mirror applyMetaAuthors' idempotent "**Authors:** …\n\n"
// content prepend so downstream Markdown still surfaces the byline up top.
func applyMetadata(result *Result, m Metadata) {
	if result == nil {
		return
	}

	if m.Author != "" {
		if result.Byline == "" || dateLikeBylineRE.MatchString(result.Byline) {
			result.Byline = m.Author
		}
		if !strings.HasPrefix(strings.TrimLeft(result.Content, " \t\n"), "**Authors:**") {
			prefix := "**Authors:** " + m.Author + "\n\n"
			result.Content = prefix + result.Content
			result.Length = len(result.Content)
		}
	}

	if result.PublishedDate == "" && m.PublishedDate != "" {
		result.PublishedDate = m.PublishedDate
	}
	if result.Language == "" && m.Language != "" {
		result.Language = m.Language
	}
	if result.Section == "" && m.Section != "" {
		result.Section = m.Section
	}
	if result.Description == "" && m.Description != "" {
		result.Description = m.Description
	}
	if result.ImageURL == "" && m.ImageURL != "" {
		result.ImageURL = m.ImageURL
	}
}
