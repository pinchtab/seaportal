package engine

import (
	"strings"

	"golang.org/x/net/html"
)

func getRole(n *html.Node) string {
	if role := getAttr(n, "role"); role != "" {
		return role
	}

	tag := strings.ToLower(n.Data)

	switch tag {
	case "header":
		return "banner"
	case "nav":
		return "navigation"
	case "main":
		return "main"
	case "footer":
		return "contentinfo"
	case "aside":
		return "complementary"
	case "section":
		if getAttr(n, "aria-label") != "" || getAttr(n, "aria-labelledby") != "" {
			return "region"
		}
		return ""
	case "article":
		return "article"
	case "form":
		return "form"

	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"

	case "a":
		if getAttr(n, "href") != "" {
			return "link"
		}
		return ""
	case "button":
		return "button"

	case "input":
		return getInputRole(n)
	case "textarea":
		return "textbox"
	case "select":
		return "combobox"
	case "option":
		return "option"

	case "ul", "ol":
		return "list"
	case "li":
		return "listitem"
	case "dl":
		return "list"
	case "dt":
		return "term"
	case "dd":
		return "definition"

	case "table":
		return "table"
	case "tr":
		return "row"
	case "th":
		return "columnheader"
	case "td":
		return "cell"
	case "thead":
		return "rowgroup"
	case "tbody":
		return "rowgroup"

	case "img":
		if getAttr(n, "alt") != "" {
			return "image"
		}
		return ""
	case "figure":
		return "figure"
	case "figcaption":
		return "caption"

	case "p":
		return "paragraph"
	case "blockquote":
		return "blockquote"
	case "pre", "code":
		return "code"

	case "details":
		return "group"
	case "summary":
		return "button"
	case "dialog":
		return "dialog"

	default:
		return ""
	}
}

func getInputRole(n *html.Node) string {
	inputType := strings.ToLower(getAttr(n, "type"))
	if inputType == "" {
		inputType = "text"
	}

	switch inputType {
	case "text", "email", "tel", "url", "password":
		return "textbox"
	case "search":
		return "searchbox"
	case "number":
		return "spinbutton"
	case "range":
		return "slider"
	case "checkbox":
		return "checkbox"
	case "radio":
		return "radio"
	case "button", "submit", "reset":
		return "button"
	case "image":
		return "button"
	default:
		return "textbox"
	}
}

func computeAccessibleName(n *html.Node) string {
	if label := getAttr(n, "aria-label"); label != "" {
		return truncateName(label)
	}

	if title := getAttr(n, "title"); title != "" {
		return truncateName(title)
	}

	tag := strings.ToLower(n.Data)

	switch tag {
	case "img":
		return truncateName(getAttr(n, "alt"))
	case "input", "textarea":
		if ph := getAttr(n, "placeholder"); ph != "" {
			return truncateName(ph)
		}
	case "a":
		return truncateName(nodeText(n, false))
	}

	return truncateName(nodeText(n, false))
}

func isInteractive(n *html.Node) bool {
	tag := strings.ToLower(n.Data)

	switch tag {
	case "a":
		return getAttr(n, "href") != ""
	case "button", "select", "textarea":
		return true
	case "input":
		inputType := strings.ToLower(getAttr(n, "type"))
		return inputType != "hidden"
	case "summary", "details":
		return true
	}

	for _, attr := range n.Attr {
		if strings.HasPrefix(attr.Key, "on") {
			return true
		}
	}

	if tabindex := getAttr(n, "tabindex"); tabindex != "" && tabindex != "-1" {
		return true
	}

	role := getAttr(n, "role")
	switch role {
	case "button", "link", "checkbox", "radio", "tab", "menuitem", "option":
		return true
	}

	return false
}

func getHeadingLevel(tag string) int {
	switch tag {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	case "h6":
		return 6
	default:
		return 0
	}
}

func truncateName(s string) string {
	s = strings.TrimSpace(s)
	s = collapseUnicodeWhitespace(s)
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}
