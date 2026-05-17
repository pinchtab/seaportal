package engine

// language.go — Lightweight stopword-frequency language detector used as a
// tail-fallback when metadata extraction (og:locale, <html lang>,
// Content-Language, DC.language, JSON-LD) leaves Result.Language empty. The
// script-block fast path handles CJK + Arabic deterministically; for
// Latin-script languages we tokenise the cleaned Markdown and count hits
// against per-language curated discriminative stopword sets.
//
// Design constraints: zero external deps, no allocation surprises, cheap
// enough to run on every extracted page. Stopword tables are curated so no
// token appears in more than two languages to keep the classifier honest.

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	langFencedCodeRE = regexp.MustCompile("(?s)```.*?```")
	langInlineCodeRE = regexp.MustCompile("`[^`]*`")
	langMDLinkRE     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// stopwords holds discriminative high-frequency function words per language.
// Lists are deliberately small (~25-30 words). A token must be exclusive to
// at most two languages to avoid cross-language false matches (e.g. "a" is
// excluded from en/es/pt/it because it appears in all four).
var stopwords = map[string]map[string]struct{}{
	"en": setOf(
		"the", "and", "of", "to", "in", "that", "is", "was", "for", "with",
		"are", "this", "but", "not", "you", "have", "from", "they", "which",
		"one", "will", "would", "there", "their", "what", "about", "who",
		"has", "been", "were",
	),
	"es": setOf(
		"que", "los", "las", "una", "por", "para", "como", "pero", "este",
		"esta", "son", "ser", "está", "han", "más", "fue", "sus", "sobre",
		"también", "entre", "mientras", "donde", "durante", "porque", "muy",
		"hasta", "desde", "cuando", "todos", "siempre",
	),
	"fr": setOf(
		"les", "une", "pour", "avec", "dans", "est", "ont", "par", "sur",
		"sont", "été", "mais", "leurs", "comme", "plus", "cette", "deux",
		"leur", "fait", "aux", "ses", "ces", "nous", "vous", "tout",
		"être", "avoir", "elle", "très", "bien",
	),
	"de": setOf(
		"der", "die", "das", "und", "ist", "sich", "nicht", "von", "mit",
		"dem", "den", "eine", "einen", "einer", "auch", "werden", "war",
		"sind", "wurde", "durch", "beim", "noch", "oder", "aber", "auf",
		"als", "ein", "wir", "ihr", "ihre",
	),
	"it": setOf(
		"che", "della", "dei", "delle", "gli", "con", "per", "sono",
		"hanno", "anche", "come", "sui", "sulla", "dalla", "dalle", "questo",
		"questa", "quello", "quella", "molto", "non", "alla", "agli", "negli",
		"nella", "essere", "stato", "fatto", "loro", "suo",
	),
	"pt": setOf(
		"dos", "das", "uma", "são", "foi", "mas", "mais", "esse", "essa",
		"depois", "também", "muito", "ser", "estão", "está", "pelo", "pela",
		"isso", "aqui", "ali", "seu", "sua", "nos", "nas", "ou",
		"então", "ainda", "havia", "tinha", "fazer",
	),
	"nl": setOf(
		"het", "een", "dat", "niet", "voor", "met", "zijn", "maar", "ook",
		"naar", "deze", "omdat", "terwijl", "tegen", "tijdens", "hadden",
		"werden", "wordt", "hebben", "wij", "jij", "hij", "haar", "zich",
		"onze", "onder", "tussen", "wel", "geen", "alles",
	),
	"ru": setOf(
		"что", "как", "для", "или", "при", "это", "был", "она", "они",
		"мне", "тебя", "вам", "его", "ему", "который", "потому", "очень",
		"между", "после", "также", "если", "когда", "уже", "ещё", "только",
		"всех", "был", "был", "был", "был",
	),
}

func setOf(words ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(words))
	for _, w := range words {
		s[w] = struct{}{}
	}
	return s
}

// DetectLanguage returns a BCP-47-ish language code (e.g. "en", "es", "ja")
// by sampling the input. Returns "" when no language scores above the
// confidence threshold. The script-block fast path handles CJK + Arabic
// deterministically; for Latin-script languages it counts stopword hits.
func DetectLanguage(markdown string) string {
	if markdown == "" {
		return ""
	}

	// Script-block fast path: scan up to first 1000 chars (by rune).
	var hira, kata, hangul, han, arabic int
	count := 0
	for _, r := range markdown {
		count++
		if count > 1000 {
			break
		}
		switch {
		case unicode.Is(unicode.Hiragana, r):
			hira++
		case unicode.Is(unicode.Katakana, r):
			kata++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Arabic, r):
			arabic++
		}
	}
	if hira+kata >= 5 {
		return "ja"
	}
	if hangul >= 5 {
		return "ko"
	}
	if han >= 5 {
		return "zh"
	}
	if arabic >= 5 {
		return "ar"
	}

	// Pre-clean: strip fenced code, inline code, and Markdown link URLs.
	cleaned := langFencedCodeRE.ReplaceAllString(markdown, " ")
	cleaned = langInlineCodeRE.ReplaceAllString(cleaned, " ")
	cleaned = langMDLinkRE.ReplaceAllString(cleaned, "$1")

	// Tokenise: split on non-letter runes; lowercase; keep tokens of len 2-15.
	tokens := strings.FieldsFunc(cleaned, func(r rune) bool {
		return !unicode.IsLetter(r)
	})

	hits := make(map[string]int, len(stopwords))
	for _, raw := range tokens {
		if len(raw) < 2 || len(raw) > 15 {
			continue
		}
		tok := strings.ToLower(raw)
		for lang, set := range stopwords {
			if _, ok := set[tok]; ok {
				hits[lang]++
			}
		}
	}

	bestLang := ""
	bestScore := 0
	tie := false
	for lang, score := range hits {
		if score > bestScore {
			bestLang = lang
			bestScore = score
			tie = false
		} else if score == bestScore && lang != bestLang {
			tie = true
		}
	}
	if bestScore < 3 || tie {
		return ""
	}
	return bestLang
}
