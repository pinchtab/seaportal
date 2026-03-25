package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// SnapshotOptions configures snapshot generation
type SnapshotOptions struct {
	FilterInteractive bool // Only include interactive elements
	MaxTokens         int  // Approximate token limit (0 = unlimited)
}

// SnapshotNode represents a node in the accessibility tree
type SnapshotNode struct {
	Role        string         `json:"role"`
	Name        string         `json:"name,omitempty"`
	Tag         string         `json:"tag,omitempty"`
	Ref         string         `json:"ref,omitempty"`
	Selector    string         `json:"selector,omitempty"`
	Depth       int            `json:"depth,omitempty"`
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
	return BuildSnapshotWithOptions(htmlStr, SnapshotOptions{})
}

// BuildSnapshotWithOptions parses HTML with configurable options
func BuildSnapshotWithOptions(htmlStr string, opts SnapshotOptions) (*SnapshotNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	ctx := &snapshotContext{
		refCounter:        0,
		filterInteractive: opts.FilterInteractive,
		tagCounts:         make(map[string]int),
	}

	root := &SnapshotNode{Role: "document", Children: []SnapshotNode{}}

	var traverse func(*html.Node, int)
	traverse = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode {
			node := ctx.buildNode(n, depth)
			if node != nil {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					ctx.traverseInto(c, node, depth+1)
				}
				// Filter: only add if interactive or has interactive children
				if ctx.filterInteractive {
					if node.Interactive || hasInteractiveChildren(node) {
						root.Children = append(root.Children, *node)
					}
				} else {
					root.Children = append(root.Children, *node)
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c, depth)
		}
	}

	traverse(doc, 0)

	// Apply max tokens if specified
	if opts.MaxTokens > 0 {
		root = truncateToTokens(root, opts.MaxTokens)
	}

	return root, nil
}

// ToCompact returns a compact text representation of the tree
func (n *SnapshotNode) ToCompact() string {
	var lines []string
	n.toCompactLines(&lines, 0)
	return strings.Join(lines, "\n")
}

func (n *SnapshotNode) toCompactLines(lines *[]string, indent int) {
	prefix := strings.Repeat("  ", indent)

	// Build line: [ref] role "name" (tag) [interactive]
	var parts []string
	if n.Ref != "" {
		parts = append(parts, n.Ref)
	}
	parts = append(parts, n.Role)
	if n.Name != "" {
		parts = append(parts, fmt.Sprintf("%q", n.Name))
	}
	if n.Tag != "" {
		parts = append(parts, fmt.Sprintf("<%s>", n.Tag))
	}
	if n.Interactive {
		parts = append(parts, "[interactive]")
	}
	if n.Href != "" {
		parts = append(parts, fmt.Sprintf("href=%s", n.Href))
	}
	if n.Level > 0 {
		parts = append(parts, fmt.Sprintf("level=%d", n.Level))
	}

	*lines = append(*lines, prefix+strings.Join(parts, " "))

	for _, child := range n.Children {
		child.toCompactLines(lines, indent+1)
	}
}

type snapshotContext struct {
	refCounter        int
	filterInteractive bool
	tagCounts         map[string]int // for selector generation
}

func (ctx *snapshotContext) traverseInto(n *html.Node, parent *SnapshotNode, depth int) {
	if n.Type == html.ElementNode {
		node := ctx.buildNode(n, depth)
		if node != nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				ctx.traverseInto(c, node, depth+1)
			}
			// Filter: only add if interactive or has interactive children
			if ctx.filterInteractive {
				if node.Interactive || hasInteractiveChildren(node) {
					parent.Children = append(parent.Children, *node)
				}
			} else {
				parent.Children = append(parent.Children, *node)
			}
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ctx.traverseInto(c, parent, depth)
	}
}

func (ctx *snapshotContext) buildNode(n *html.Node, depth int) *SnapshotNode {
	role := getRole(n)
	if role == "" {
		return nil
	}

	ctx.refCounter++
	tag := strings.ToLower(n.Data)

	node := &SnapshotNode{
		Role:        role,
		Tag:         tag,
		Ref:         fmt.Sprintf("e%d", ctx.refCounter),
		Selector:    ctx.buildSelector(n),
		Depth:       depth,
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

func (ctx *snapshotContext) buildSelector(n *html.Node) string {
	tag := strings.ToLower(n.Data)

	// Priority 1: ID selector
	if id := snapshotGetAttr(n, "id"); id != "" {
		return "#" + id
	}

	// Priority 2: Unique class selector
	if class := snapshotGetAttr(n, "class"); class != "" {
		classes := strings.Fields(class)
		if len(classes) > 0 {
			// Use first meaningful class
			for _, c := range classes {
				if !strings.HasPrefix(c, "js-") && len(c) > 1 {
					return tag + "." + c
				}
			}
			return tag + "." + classes[0]
		}
	}

	// Priority 3: Tag with nth-of-type
	ctx.tagCounts[tag]++
	return fmt.Sprintf("%s:nth-of-type(%d)", tag, ctx.tagCounts[tag])
}

func hasInteractiveChildren(n *SnapshotNode) bool {
	for _, child := range n.Children {
		if child.Interactive || hasInteractiveChildren(&child) {
			return true
		}
	}
	return false
}

func truncateToTokens(root *SnapshotNode, maxTokens int) *SnapshotNode {
	// Rough estimate: 4 chars per token
	maxChars := maxTokens * 4

	// Serialize to check size
	data, _ := json.Marshal(root)
	if len(data) <= maxChars {
		return root
	}

	// Truncate by removing children from deepest levels
	result := *root
	result.Children = truncateChildren(root.Children, maxChars, len(data))
	return &result
}

func truncateChildren(children []SnapshotNode, maxChars, currentSize int) []SnapshotNode {
	if currentSize <= maxChars || len(children) == 0 {
		return children
	}

	// Remove children from the end
	result := make([]SnapshotNode, 0, len(children))
	for i, child := range children {
		childData, _ := json.Marshal(child)
		childSize := len(childData)

		if currentSize-childSize > maxChars && i > len(children)/2 {
			currentSize -= childSize
			continue
		}

		// Recursively truncate this child's children
		truncated := child
		truncated.Children = truncateChildren(child.Children, maxChars/2, childSize)
		result = append(result, truncated)
	}

	return result
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
