package engine

import "strings"

func findMatchingClose(html string, openEnd int, tagName string) int {
	depth := 1
	i := openEnd
	n := len(html)
	for i < n {
		lt := strings.IndexByte(html[i:], '<')
		if lt < 0 {
			return -1
		}
		i += lt
		if i+1 >= n {
			return -1
		}
		if html[i+1] == '/' {
			if hasTagPrefix(html[i+2:], tagName) {
				gt := strings.IndexByte(html[i:], '>')
				if gt < 0 {
					return -1
				}
				depth--
				if depth == 0 {
					return i + gt + 1
				}
				i += gt + 1
				continue
			}
			gt := strings.IndexByte(html[i:], '>')
			if gt < 0 {
				return -1
			}
			i += gt + 1
			continue
		}
		if hasTagPrefix(html[i+1:], tagName) {
			tagEnd, selfClosing := scanTagEnd(html, i)
			if tagEnd < 0 {
				return -1
			}
			if !selfClosing && !isVoidElement(tagName) {
				depth++
			}
			i = tagEnd
			continue
		}
		gt := strings.IndexByte(html[i:], '>')
		if gt < 0 {
			return -1
		}
		i += gt + 1
	}
	return -1
}

func hasTagPrefix(s, tagName string) bool {
	if len(s) < len(tagName) {
		return false
	}
	if !strings.EqualFold(s[:len(tagName)], tagName) {
		return false
	}
	if len(s) == len(tagName) {
		return true
	}
	return isAttrTerminator(s[len(tagName)])
}

func scanTagEnd(html string, i int) (int, bool) {
	j := i + 1
	n := len(html)
	for j < n {
		switch c := html[j]; c {
		case '"', '\'':
			k := strings.IndexByte(html[j+1:], c)
			if k < 0 {
				return -1, false
			}
			j += k + 2
		case '>':
			selfClosing := j > i+1 && html[j-1] == '/'
			return j + 1, selfClosing
		default:
			j++
		}
	}
	return -1, false
}

var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}

func isVoidElement(tag string) bool {
	return voidElements[tag]
}

var rawTextElements = map[string]bool{
	"script":   true,
	"style":    true,
	"textarea": true,
	"title":    true,
}

func rawTextClose(html string, contentStart int, tagName string) (closeStart, closeEnd int) {
	n := len(html)
	i := contentStart
	for i < n {
		lt := strings.IndexByte(html[i:], '<')
		if lt < 0 {
			return n, n
		}
		i += lt
		if i+1 < n && html[i+1] == '/' && hasTagPrefix(html[i+2:], tagName) {
			gt := strings.IndexByte(html[i:], '>')
			if gt < 0 {
				return n, n
			}
			return i, i + gt + 1
		}
		i++
	}
	return n, n
}

func hasHiddenBoolAttr(attrs string) bool {
	i := 0
	for i < len(attrs) {
		c := attrs[i]
		switch c {
		case '"', '\'':
			j := strings.IndexByte(attrs[i+1:], c)
			if j < 0 {
				return false
			}
			i += j + 2
		case ' ', '\t', '\n', '\r', '/':
			i++
		default:
			start := i
			for i < len(attrs) && !isAttrTerminator(attrs[i]) && attrs[i] != '=' {
				i++
			}
			name := strings.ToLower(attrs[start:i])
			for i < len(attrs) && (attrs[i] == ' ' || attrs[i] == '\t' || attrs[i] == '\n' || attrs[i] == '\r') {
				i++
			}
			if i < len(attrs) && attrs[i] == '=' {
				i++
				for i < len(attrs) && (attrs[i] == ' ' || attrs[i] == '\t') {
					i++
				}
				if i < len(attrs) && (attrs[i] == '"' || attrs[i] == '\'') {
					q := attrs[i]
					j := strings.IndexByte(attrs[i+1:], q)
					if j < 0 {
						return false
					}
					i += j + 2
				} else {
					for i < len(attrs) && !isAttrTerminator(attrs[i]) {
						i++
					}
				}
			}
			if name == "hidden" {
				return true
			}
		}
	}
	return false
}

func isAttrTerminator(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' || b == '>'
}

func removeElementsSinglePass(html string, shouldRemove func(tagName, attrs string) bool) string {
	var out strings.Builder
	out.Grow(len(html))
	pos := 0
	n := len(html)
	for pos < n {
		lt := strings.IndexByte(html[pos:], '<')
		if lt < 0 {
			out.WriteString(html[pos:])
			break
		}
		tagStartAbs := pos + lt

		if tagStartAbs+1 >= n {
			out.WriteString(html[pos:])
			break
		}
		c := html[tagStartAbs+1]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}

		nameStart := tagStartAbs + 1
		nameEnd := nameStart
		for nameEnd < n {
			ch := html[nameEnd]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
				nameEnd++
				continue
			}
			break
		}
		if nameEnd == nameStart {
			out.WriteString(html[pos : tagStartAbs+1])
			pos = tagStartAbs + 1
			continue
		}

		tagEnd, selfClosing := scanTagEnd(html, tagStartAbs)
		if tagEnd < 0 {
			out.WriteString(html[pos:])
			break
		}

		attrs := ""
		if nameEnd < tagEnd-1 {
			attrs = html[nameEnd : tagEnd-1]
			if selfClosing && len(attrs) > 0 && attrs[len(attrs)-1] == '/' {
				attrs = attrs[:len(attrs)-1]
			}
		}

		tagName := strings.ToLower(html[nameStart:nameEnd])

		if rawTextElements[tagName] && !selfClosing {
			_, closeEnd := rawTextClose(html, tagEnd, tagName)
			if shouldRemove(tagName, attrs) {
				out.WriteString(html[pos:tagStartAbs])
			} else {
				out.WriteString(html[pos:closeEnd])
			}
			pos = closeEnd
			continue
		}

		if !shouldRemove(tagName, attrs) {
			out.WriteString(html[pos:tagEnd])
			pos = tagEnd
			continue
		}

		out.WriteString(html[pos:tagStartAbs])

		if selfClosing || isVoidElement(tagName) {
			pos = tagEnd
			continue
		}
		closeEnd := findMatchingClose(html, tagEnd, tagName)
		if closeEnd < 0 {
			pos = tagEnd
			continue
		}
		pos = closeEnd
	}
	return out.String()
}
