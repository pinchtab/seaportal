package engine

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	pdfReader "github.com/ledongthuc/pdf"
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
		text, terr := safeGetPlainText(page)
		if terr != nil {
			if i > 1 {
				sb.WriteString("\n\n")
			}
			fmt.Fprintf(&sb, "--- page %d ---\n\n", i)
			fmt.Fprintf(&sb, "[extraction failed: %v]\n", terr)
			continue
		}
		if i > 1 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "--- page %d ---\n\n", i)
		sb.WriteString(strings.TrimSpace(text))
		sb.WriteString("\n")
	}
	out = strings.TrimSpace(sb.String())
	if out == "" {
		return "", fmt.Errorf("pdf yielded no text (possibly image-only/scanned)")
	}
	out = collapseBlankRunsRe.ReplaceAllString(out, "\n\n")
	return out, nil
}

func safeGetPlainText(page pdfReader.Page) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during page text extraction: %v", r)
		}
	}()
	return page.GetPlainText(nil)
}
