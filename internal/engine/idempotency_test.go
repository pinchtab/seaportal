package engine

import (
	"testing"
)

func firstDiff(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	idx := -1
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			idx = i
			break
		}
	}
	if idx == -1 {
		if len(a) == len(b) {
			return ""
		}
		idx = n
	}
	const window = 40
	lo := idx - window
	if lo < 0 {
		lo = 0
	}
	hiA := idx + window
	if hiA > len(a) {
		hiA = len(a)
	}
	hiB := idx + window
	if hiB > len(b) {
		hiB = len(b)
	}
	return "@" + itoa(idx) + " a=" + safeSlice(a, lo, hiA) + " | b=" + safeSlice(b, lo, hiB)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func safeSlice(s string, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	if lo >= hi {
		return ""
	}
	return s[lo:hi]
}

func TestIdempotency(t *testing.T) {
	latin1 := loadFixture(t, "static/charset-latin1.html")
	mdn := loadFixture(t, "ssr/mdn-http-methods.html")

	htmlCases := []struct{ name, in string }{
		{"empty", ""},
		{"minimal", "<p>hello</p>"},
		{"static-fixture", latin1},
		{"ssr-fixture", mdn},
	}
	mdCases := []struct{ name, in string }{
		{"empty", ""},
		{"minimal", "# Title\n\nBody."},
		{"with-noise", "# Title\n\n\n\n\n\nBody"},
		{"with-dups", "Hello world\n\nHello world\n\nUnique"},
	}

	nonIdempotent := map[string]bool{}

	check := func(t *testing.T, key, name string, out1, out2 string) {
		t.Helper()
		if out1 == out2 {
			return
		}
		msg := name + " NOT idempotent: len1=" + itoa(len(out1)) +
			" len2=" + itoa(len(out2)) +
			" delta=" + itoa(len(out1)-len(out2)) +
			" first divergence: " + firstDiff(out1, out2)
		if nonIdempotent[key] {
			t.Logf("observed-non-idempotent [%s]: %s", key, msg)
			return
		}
		t.Errorf("%s", msg)
	}

	t.Run("PreprocessHTML", func(t *testing.T) {
		for _, c := range htmlCases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				out1 := PreprocessHTML(c.in)
				out2 := PreprocessHTML(out1)
				check(t, "PreprocessHTML/"+c.name, c.name, out1, out2)
			})
		}
	})

	t.Run("SanitizeHTML", func(t *testing.T) {
		for _, c := range htmlCases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				out1 := SanitizeHTML(c.in)
				out2 := SanitizeHTML(out1)
				check(t, "SanitizeHTML/"+c.name, c.name, out1, out2)
			})
		}
	})

	t.Run("CleanupMarkdown", func(t *testing.T) {
		for _, c := range mdCases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				out1 := CleanupMarkdown(c.in)
				out2 := CleanupMarkdown(out1)
				check(t, "CleanupMarkdown/"+c.name, c.name, out1, out2)
			})
		}
	})

	t.Run("Dedupe", func(t *testing.T) {
		for _, c := range mdCases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				out1 := Dedupe(c.in).Content
				out2 := Dedupe(out1).Content
				check(t, "Dedupe/"+c.name, c.name, out1, out2)
			})
		}
	})
}
