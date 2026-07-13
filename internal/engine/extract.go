package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	mdtable "github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/andybalholm/brotli"
	"github.com/go-shiori/go-readability"
	"github.com/klauspost/compress/zstd"
)

var (
	mdConverterOnce sync.Once
	mdConverter     *converter.Converter
)

func getMarkdownConverter() *converter.Converter {
	mdConverterOnce.Do(func() {
		mdConverter = converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(),
				mdtable.NewTablePlugin(),
			),
		)
	})
	return mdConverter
}

func convertHTMLToMarkdown(html string) (string, error) {
	return getMarkdownConverter().ConvertString(html)
}

func FromURL(targetURL string) Result {
	return FromURLWithOptions(targetURL, Options{})
}

func FromURLWithDedupe(targetURL string) Result {
	return FromURLWithOptions(targetURL, Options{Dedupe: true})
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

var ErrNeedsBrowser = errors.New("needs-browser")

const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const DefaultAcceptEncoding = "gzip, deflate, br, zstd"

const DefaultAccept = "text/markdown, text/html;q=0.9, application/xhtml+xml;q=0.8, application/xml;q=0.7, */*;q=0.1"

func newGETRequest(targetURL string, userAgent string, requestID string, sendRequestID bool) *http.Request {
	req, _ := http.NewRequest("GET", targetURL, nil)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", DefaultAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", DefaultAcceptEncoding)
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if sendRequestID && requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func FromURLContext(ctx context.Context, targetURL string, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	opts.Context = ctx
	return fromURLWithOptions(targetURL, opts)
}

func FromURLWithOptions(targetURL string, opts Options) Result {
	return FromURLContext(opts.Context, targetURL, opts)
}

func fromURLWithOptions(targetURL string, opts Options) (result Result) {
	if opts.HeadOnly {
		return fetchHeadOnly(targetURL, opts)
	}
	defer ensureProfile(&result)
	start := time.Now()
	result = Result{URL: targetURL}

	st, ok := fetchDocument(targetURL, opts, start, &result)
	defer func() {
		if st.resp != nil && st.resp.Body != nil {
			_ = st.resp.Body.Close()
		}
	}()
	if !ok {
		return result
	}

	if strings.Contains(strings.ToLower(st.respContentType), "application/pdf") {
		extractPDF(&result, targetURL, opts, &st, start)
		return result
	}

	if isRawTextContentType(st.respContentType) {
		extractRawText(&result, targetURL, opts, &st, start)
		return result
	}

	html := string(st.bodyBytes)
	contentLength := int64(len(st.bodyBytes))
	fetchTimeMs := time.Since(start).Milliseconds()
	negotiationFailed := st.resp.StatusCode == http.StatusNotFound || st.resp.StatusCode == http.StatusNotAcceptable
	if negotiationFailed && !strings.Contains(st.respContentType, "text/markdown") {
		if !renegotiateHTML(targetURL, opts, &st, &result) {
			return result
		}
		html = string(st.bodyBytes)
		contentLength = int64(len(st.bodyBytes))
		fetchTimeMs = time.Since(start).Milliseconds()
	}

	if strings.Contains(st.respContentType, "text/markdown") {
		extractNegotiatedMarkdown(&result, opts, &st, html, contentLength, fetchTimeMs, start)
		return result
	}

	if opts.FastMode {
		needsBrowser, reason := QuickNeedsBrowser(html)
		if needsBrowser {
			result.TimeMs = time.Since(start).Milliseconds()
			result.FetchTimeMs = fetchTimeMs
			result.StatusCode = st.resp.StatusCode
			result.ContentLength = contentLength
			result.Validation = Validation{
				IsValid:      false,
				NeedsBrowser: true,
				Confidence:   0.1,
				Issues:       []string{reason},
			}
			result.setError(fmt.Errorf("%w: %s", ErrNeedsBrowser, reason))
			applyStatusBlockedProfile(&result, st.resp.StatusCode)
			return result
		}
	}

	detectedCharset := result.Charset
	cacheHit := result.CacheHit
	cacheRevalidated := result.CacheRevalidated
	cacheStale := result.CacheStale
	protocol := result.Protocol
	result = fromHTMLInternal(html, targetURL, start, opts)
	result.Charset = detectedCharset
	result.CacheHit = cacheHit
	result.CacheRevalidated = cacheRevalidated
	result.CacheStale = cacheStale
	result.Protocol = protocol
	result.FetchTimeMs = fetchTimeMs

	finalizeTransport(&result, opts, &st, contentLength)

	if st.resp.StatusCode == http.StatusOK {
		isSoft404, soft404Hints := detectSoft404(html, contentLength)
		result.IsSoft404 = isSoft404
		result.Soft404Hints = soft404Hints
	}

	applyStatusBlockedProfile(&result, st.resp.StatusCode)

	return result
}

func applyStatusBlockedProfile(result *Result, statusCode int) {
	var blockedReason string
	switch statusCode {
	case http.StatusUnauthorized:
		blockedReason = "http-401-unauthorized"
	case http.StatusForbidden:
		blockedReason = "http-403-forbidden"
	case http.StatusTooManyRequests:
		blockedReason = "http-429-rate-limited"
	case http.StatusBadGateway:
		blockedReason = "http-502-bad-gateway"
	case http.StatusServiceUnavailable:
		blockedReason = "http-503-service-unavailable"
	case http.StatusGatewayTimeout:
		blockedReason = "http-504-gateway-timeout"
	}
	if blockedReason != "" {
		result.IsBlocked = true
		result.Profile.Class = PageBlocked
		result.Profile.Outcome = OutcomeNeedsBrowser
		result.Profile.Trustworthy = false
		result.Profile.Reasons = append(result.Profile.Reasons, blockedReason)
		result.PageClass = PageBlocked
		return
	}
	switch {
	case statusCode == http.StatusNotFound:
		result.Profile.Reasons = append(result.Profile.Reasons, "http-404-not-found")
	case statusCode >= 500 && statusCode < 600:
		result.Profile.Reasons = append(result.Profile.Reasons, "http-5xx-server-error")
	}
}

func FromHTML(html string, targetURL string) Result {
	start := time.Now()
	return fromHTMLInternal(html, targetURL, start, Options{})
}

func FromHTMLWithOptions(html string, targetURL string, opts Options) Result {
	start := time.Now()
	return fromHTMLInternal(html, targetURL, start, opts)
}

func FromResponse(resp *http.Response, targetURL string, start time.Time) (result Result) {
	defer ensureProfile(&result)
	result = Result{URL: targetURL}

	parsedURL, _ := url.Parse(targetURL)
	parseStart := time.Now()
	article, err := readability.FromReader(resp.Body, parsedURL)
	parseEnd := time.Now()
	if err != nil {
		result.setError(err)
		return result
	}

	return processArticle(article, targetURL, start, parseStart, parseEnd)
}

func fromHTMLInternal(html string, targetURL string, start time.Time, opts Options) (result Result) {
	defer ensureProfile(&result)

	var selectorWarnings []string
	if opts.SelectCSS != "" || opts.StripCSS != "" {
		modified, warns := applySelectorOps(html, opts.SelectCSS, opts.StripCSS)
		html = modified
		selectorWarnings = warns
	}

	spaSignals, isSPA := DetectSPA(html)
	isBlocked := DetectBlocked(html)
	if !isBlocked && DetectJSChallenge(html, "text/html", len(html)) {
		isBlocked = true
		spaSignals = append(spaSignals, "js-challenge-200ok")
	}

	parsedURL, _ := url.Parse(targetURL)

	canonicalPick := PickCanonical(targetURL, html)

	var extractedLinks []LinkRef
	if opts.WithLinks {
		extractedLinks = ExtractLinks(html, targetURL)
	}

	var extractedImages []ImageRef
	if opts.WithImages {
		extractedImages = ExtractImages(html, targetURL)
	}

	var extractedTables []TableRef
	if opts.WithTables {
		extractedTables = ExtractTables(html, targetURL)
	}

	var extractedComments []CommentRef
	if opts.WithComments {
		extractedComments = ExtractComments(html, targetURL)
	}

	var extractedSchema map[string]interface{}
	var schemaWarnings []string
	schema := opts.Schema
	if schema == nil && opts.SchemaPath != "" {
		s, err := LoadSchema(opts.SchemaPath)
		if err != nil {
			schemaWarnings = append(schemaWarnings, "schema load failed: "+err.Error())
		} else {
			schema = &s
		}
	}
	if schema != nil {
		if len(schema.Fields) == 0 {
			schemaWarnings = append(schemaWarnings, "schema loaded but has no fields; expected a top-level 'fields' map")
		} else {
			extracted, err := ApplySchema(html, *schema)
			if err != nil {
				schemaWarnings = append(schemaWarnings, "schema apply failed: "+err.Error())
			} else {
				extractedSchema = extracted
			}
		}
	}

	rawHTML := html

	html = PreprocessHTMLWithURL(html, parsedURL)

	ldBlocks := ExtractLDJSON(html)

	pageMetadata := ExtractMetadata(html)

	html = SanitizeHTML(html)
	parseStart := time.Now()
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	parseEnd := time.Now()
	if err != nil {
		result = Result{URL: targetURL, SPASignals: spaSignals, IsSPA: isSPA, IsBlocked: isBlocked}
		result.setError(err)
		return result
	}

	result = processArticle(article, targetURL, start, parseStart, parseEnd)

	applyPruneFallback(&result, rawHTML, parsedURL, opts)

	if !opts.NoPruneFallback {
		applyPreprocessSkipFallback(&result, html, rawHTML, targetURL, parsedURL, start, parseStart, parseEnd)
	}

	result.SPASignals = spaSignals
	result.IsSPA = isSPA
	result.IsBlocked = isBlocked

	applyLDJSONMetadata(&result, ldBlocks)

	applyMetadata(&result, pageMetadata)

	if result.Language == "" && len(result.Content) > 200 {
		result.Language = DetectLanguage(result.Content)
	}

	result.Confidence = computeConfidence(confidenceInputsFrom(&result))

	applyIndexPageFallback(&result, html)

	applyLDJSONArticleBodyFallback(&result, ldBlocks, opts)

	applyTextFallback(&result, html)

	applyLDJSONSupplement(&result, ldBlocks)

	if result.Confidence < 30 {
		result.IsSPA = true
	}

	if content, applied := applyLinkRetentionStage(result.Content, opts); applied {
		result.Content = content
		result.Length = len(result.Content)
	}

	if content, applied := applyDedupeStage(result.Content, &result, opts); applied {
		result.Content = content
		result.Length = len(result.Content)
		refreshContentMetrics(&result)
	}

	result.HasLLMContent = detectLLMContent(result.Content)

	result.Profile = ClassifyPage(result)

	applyProbeSearchOverride(&result, opts)

	if opts.FailFast && result.IsSPA && result.Confidence < 30 {
		result.setError(fmt.Errorf("SPA detected with low confidence (%d%%), signals: %v", result.Confidence, result.SPASignals))
	}

	if canonicalPick != "" && canonicalPick != result.URL {
		result.CanonicalURL = canonicalPick
	}

	if opts.WithLinks {
		result.Links = extractedLinks
	}

	if opts.WithImages {
		result.Images = extractedImages
	}

	if opts.WithTables {
		result.Tables = extractedTables
	}

	if opts.WithComments {
		result.Comments = extractedComments
	}

	if len(selectorWarnings) > 0 {
		result.Warnings = append(result.Warnings, selectorWarnings...)
	}

	if extractedSchema != nil {
		result.Schema = extractedSchema
	}
	if len(schemaWarnings) > 0 {
		result.Warnings = append(result.Warnings, schemaWarnings...)
	}

	if content, applied := applyTruncateStage(result.Content, &result, opts); applied {
		result.Content = content
		result.Length = len(result.Content)
	}

	applyChunkStage(result.Content, &result, opts)

	applyQueryRanking(&result, opts)

	return result
}

func applyQueryRanking(result *Result, opts Options) {
	if strings.TrimSpace(opts.Query) == "" {
		return
	}
	ranked := RankSections(result.Content, opts.Query, 1.5, 0.75, opts.TopN)
	if len(ranked) == 0 {
		return
	}
	result.RankedSections = ranked
	if !opts.FilterByQuery {
		return
	}
	limit := opts.TopN
	if limit <= 0 {
		limit = 3
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}
	var sb strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if ranked[i].Heading != "" {
			if !strings.HasPrefix(ranked[i].Text, ranked[i].Heading) {
				sb.WriteString(ranked[i].Heading)
				sb.WriteString("\n\n")
			}
		}
		sb.WriteString(ranked[i].Text)
	}
	result.Content = sb.String()
	result.Length = len(result.Content)
}

func processArticle(article readability.Article, targetURL string, start time.Time, parseStart time.Time, parseEnd time.Time) (result Result) {
	defer ensureProfile(&result)
	result = Result{URL: targetURL}

	convertStart := time.Now()
	markdown, err := convertHTMLToMarkdown(article.Content)
	convertEnd := time.Now()
	if err != nil {
		result.setError(err)
		return result
	}

	markdown = CleanupMarkdown(markdown)

	result.Title = article.Title
	result.Content = markdown
	result.Byline = article.Byline
	result.Excerpt = article.Excerpt
	result.SiteName = article.SiteName
	result.Length = len(markdown)
	result.TimeMs = time.Since(start).Milliseconds()
	result.ParseTimeMs = parseEnd.Sub(parseStart).Milliseconds()
	result.ConvertTimeMs = convertEnd.Sub(convertStart).Milliseconds()

	result.HeadingCount = CountPattern(article.Content, `<h[1-6]`)
	htmlLinks := CountPattern(article.Content, `<a\s`)
	mdLinks := CountMarkdownLinks(markdown)
	if mdLinks > htmlLinks {
		result.LinkCount = mdLinks
	} else {
		result.LinkCount = htmlLinks
	}
	result.ParagraphCount = CountPattern(article.Content, `<p[\s>]`)

	result.Confidence = computeConfidence(confidenceInputs{length: result.Length, headingCount: result.HeadingCount, paragraphCount: result.ParagraphCount})

	result.QualityInfo = ComputeQuality(markdown)
	result.Quality = result.QualityInfo.Score
	result.Fingerprint = SemanticFingerprint(markdown)
	result.Validation = ValidateExtraction(&result)
	result.ExtractionMethod = "readability"

	return result
}

func decompressBody(data []byte, encoding string) ([]byte, error) {
	return decompressBodyLimited(data, encoding, 0)
}

func decompressBodyLimited(data []byte, encoding string, max int64) ([]byte, error) {
	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()
		return limitedReadAll(reader, max, ErrDecompressTooLarge)

	case "deflate":
		reader := flate.NewReader(bytes.NewReader(data))
		defer func() { _ = reader.Close() }()
		return limitedReadAll(reader, max, ErrDecompressTooLarge)

	case "br":
		reader := brotli.NewReader(bytes.NewReader(data))
		return limitedReadAll(reader, max, ErrDecompressTooLarge)

	case "zstd":
		reader, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return limitedReadAll(reader, max, ErrDecompressTooLarge)

	case "identity", "":
		return data, nil

	default:
		return data, nil
	}
}

func ExtractFromHTML(html string, targetURL string) (string, error) {
	parsedURL, _ := url.Parse(targetURL)
	html = PreprocessHTMLWithURL(html, parsedURL)
	article, err := readability.FromReader(strings.NewReader(html), parsedURL)
	if err != nil {
		return "", err
	}

	markdown, err := convertHTMLToMarkdown(article.Content)
	if err != nil {
		return article.TextContent, nil
	}
	return markdown, nil
}
