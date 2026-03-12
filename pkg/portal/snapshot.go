package portal

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// SnapshotNode represents a node in the accessibility tree
type SnapshotNode struct {
	Role        string         `json:"role"`
	Name        string         `json:"name,omitempty"`
	Ref         string         `json:"ref,omitempty"`
	Interactive bool           `json:"interactive,omitempty"`
	Level       int            `json:"level,omitempty"`
	Value       string         `json:"value,omitempty"`
	Href        string         `json:"href,omitempty"`
	Checked     *bool          `json:"checked,omitempty"`
	Disabled    bool           `json:"disabled,omitempty"`
	Children    []SnapshotNode `json:"children,omitempty"`
}

// BuildSnapshot parses HTML and returns an accessibility tree
func BuildSnapshot(htmlStr string) (*SnapshotNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	refCounter := 0
	root := &SnapshotNode{Role: "document", Children: []SnapshotNode{}}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			node := buildNode(n, &refCounter)
			if node != nil {
				// Traverse children
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					childBefore := len(node.Children)
					traverseInto(c, node, &refCounter)
					_ = childBefore
				}
				root.Children = append(root.Children, *node)
				return
			}
		}
		// Continue traversing
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	return root, nil
}

func traverseInto(n *html.Node, parent *SnapshotNode, refCounter *int) {
	if n.Type == html.ElementNode {
		node := buildNode(n, refCounter)
		if node != nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				traverseInto(c, node, refCounter)
			}
			parent.Children = append(parent.Children, *node)
			return
		}
	}
	// Continue to children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		traverseInto(c, parent, refCounter)
	}
}

func buildNode(n *html.Node, refCounter *int) *SnapshotNode {
	role := getRole(n)
	if role == "" {
		return nil
	}

	*refCounter++
	node := &SnapshotNode{
		Role:        role,
		Ref:         fmt.Sprintf("e%d", *refCounter),
		Name:        computeAccessibleName(n),
		Interactive: isInteractive(n),
		Children:    []SnapshotNode{},
	}

	// Add role-specific attributes
	switch role {
	case "heading":
		node.Level = getHeadingLevel(n.Data)
	case "link":
		node.Href = snapshotGetAttr(n, "href")
	case "textbox", "searchbox":
		node.Value = snapshotGetAttr(n, "value")
	case "checkbox", "radio":
		checked := snapshotHasAttr(n, "checked")
		node.Checked = &checked
	}

	if snapshotHasAttr(n, "disabled") {
		node.Disabled = true
	}

	return node
}

func getRole(n *html.Node) string {
	// Explicit ARIA role takes precedence
	if role := snapshotGetAttr(n, "role"); role != "" {
		return role
	}

	// Map tag to implicit role
	tag := strings.ToLower(n.Data)

	switch tag {
	// Landmarks
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
		if snapshotGetAttr(n, "aria-label") != "" || snapshotGetAttr(n, "aria-labelledby") != "" {
			return "region"
		}
		return ""
	case "article":
		return "article"
	case "form":
		return "form"

	// Headings
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"

	// Links and buttons
	case "a":
		if snapshotGetAttr(n, "href") != "" {
			return "link"
		}
		return ""
	case "button":
		return "button"

	// Form controls
	case "input":
		return getInputRole(n)
	case "textarea":
		return "textbox"
	case "select":
		return "combobox"
	case "option":
		return "option"

	// Lists
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

	// Tables
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

	// Media
	case "img":
		if snapshotGetAttr(n, "alt") != "" {
			return "image"
		}
		return "" // decorative image
	case "figure":
		return "figure"
	case "figcaption":
		return "caption"

	// Text structure
	case "p":
		return "paragraph"
	case "blockquote":
		return "blockquote"
	case "pre", "code":
		return "code"

	// Interactive
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
	inputType := strings.ToLower(snapshotGetAttr(n, "type"))
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
	// Priority 1: aria-label
	if label := snapshotGetAttr(n, "aria-label"); label != "" {
		return truncateName(label)
	}

	// Priority 2: title attribute
	if title := snapshotGetAttr(n, "title"); title != "" {
		return truncateName(title)
	}

	// Priority 3: Element-specific
	tag := strings.ToLower(n.Data)

	switch tag {
	case "img":
		return truncateName(snapshotGetAttr(n, "alt"))
	case "input", "textarea":
		if ph := snapshotGetAttr(n, "placeholder"); ph != "" {
			return truncateName(ph)
		}
	case "a":
		// Use link text
		return truncateName(snapshotGetTextContent(n))
	}

	// Priority 4: Text content
	return truncateName(snapshotGetTextContent(n))
}

func isInteractive(n *html.Node) bool {
	tag := strings.ToLower(n.Data)

	// Inherently interactive elements
	switch tag {
	case "a":
		return snapshotGetAttr(n, "href") != ""
	case "button", "select", "textarea":
		return true
	case "input":
		inputType := strings.ToLower(snapshotGetAttr(n, "type"))
		return inputType != "hidden"
	case "summary", "details":
		return true
	}

	// Check for event handlers
	for _, attr := range n.Attr {
		if strings.HasPrefix(attr.Key, "on") {
			return true
		}
	}

	// Check tabindex
	if tabindex := snapshotGetAttr(n, "tabindex"); tabindex != "" && tabindex != "-1" {
		return true
	}

	// Check role
	role := snapshotGetAttr(n, "role")
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

func snapshotGetAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func snapshotHasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func snapshotGetTextContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

func truncateName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ") // normalize whitespace
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}
