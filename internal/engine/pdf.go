package engine

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	pdfReader "github.com/ledongthuc/pdf"
	"golang.org/x/text/unicode/norm"
)

var collapseBlankRunsRe = regexp.MustCompile(`\n{3,}`)

func ExtractPDFText(body []byte) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("pdf: panic during parse: %v", r)
		}
	}()
	if len(body) == 0 {
		return "", fmt.Errorf("empty PDF body")
	}
	r, readerErr := pdfReader.NewReader(bytes.NewReader(body), int64(len(body)))
	if readerErr != nil {
		return "", fmt.Errorf("pdf reader: %w", readerErr)
	}
	n := r.NumPage()
	if n == 0 {
		return "", fmt.Errorf("pdf has zero pages")
	}
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, terr := safeGetPageText(page)
		if i > 1 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "--- page %d ---\n\n", i)
		if terr != nil {
			fmt.Fprintf(&sb, "[extraction failed: %v]\n", terr)
			continue
		}
		sb.WriteString(strings.TrimSpace(text))
		sb.WriteString("\n")
	}
	out = strings.TrimSpace(sb.String())
	if out == "" {
		return "", fmt.Errorf("pdf yielded no text (possibly image-only/scanned)")
	}
	out = collapseBlankRunsRe.ReplaceAllString(out, "\n\n")
	return normalizePDFText(out), nil
}

func safeGetPageText(page pdfReader.Page) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during page text extraction: %v", r)
		}
	}()
	if t := layoutText(page.Content().Text); strings.TrimSpace(t) != "" {
		return t, nil
	}
	return page.GetPlainText(nil)
}

func layoutText(els []pdfReader.Text) string {
	if len(els) == 0 {
		return ""
	}
	buckets := map[int64][]pdfReader.Text{}
	var keys []int64
	for _, t := range els {
		if t.S == "" {
			continue
		}
		key := int64(math.Round(t.Y))
		if _, ok := buckets[key]; !ok {
			keys = append(keys, key)
		}
		buckets[key] = append(buckets[key], t)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		row := buckets[k]
		sort.SliceStable(row, func(i, j int) bool { return row[i].X < row[j].X })
		if line := strings.TrimRight(assembleRow(row), " "); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func assembleRow(runs []pdfReader.Text) string {
	var b strings.Builder
	var prev *pdfReader.Text
	for i := range runs {
		if runs[i].S == "" {
			continue
		}
		cur := &runs[i]
		if prev != nil {
			gap := cur.X - (prev.X + prev.W)
			threshold := 0.2 * prev.FontSize
			if threshold <= 0 {
				threshold = 1.0
			}
			if gap > threshold && !strings.HasSuffix(prev.S, " ") && !strings.HasPrefix(cur.S, " ") {
				b.WriteByte(' ')
			}
		}
		b.WriteString(cur.S)
		prev = cur
	}
	return b.String()
}

func normalizePDFText(s string) string {
	return norm.NFKC.String(s)
}
