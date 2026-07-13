package engine

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

type SnapshotOptions struct {
	FilterInteractive bool
	MaxTokens         int
}

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

func BuildSnapshot(htmlStr string) (*SnapshotNode, error) {
	return BuildSnapshotWithOptions(htmlStr, SnapshotOptions{})
}

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

	ctx.traverseInto(doc, root, 0)

	if opts.MaxTokens > 0 {
		root = truncateToTokens(root, opts.MaxTokens)
	}

	return root, nil
}

type snapshotContext struct {
	refCounter        int
	filterInteractive bool
	tagCounts         map[string]int
}

func (ctx *snapshotContext) traverseInto(n *html.Node, parent *SnapshotNode, depth int) {
	if n.Type == html.ElementNode {
		node := ctx.buildNode(n, depth)
		if node != nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				ctx.traverseInto(c, node, depth+1)
			}
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

	switch role {
	case "heading":
		node.Level = getHeadingLevel(n.Data)
	case "link":
		node.Href = getAttr(n, "href")
	case "textbox", "searchbox":
		node.Value = getAttr(n, "value")
	case "checkbox", "radio":
		checked := hasAttr(n, "checked")
		node.Checked = &checked
	}

	if hasAttr(n, "disabled") {
		node.Disabled = true
	}

	return node
}

func (ctx *snapshotContext) buildSelector(n *html.Node) string {
	tag := strings.ToLower(n.Data)

	if id := getAttr(n, "id"); id != "" {
		return "#" + id
	}

	if class := getAttr(n, "class"); class != "" {
		classes := strings.Fields(class)
		if len(classes) > 0 {
			for _, c := range classes {
				if !strings.HasPrefix(c, "js-") && len(c) > 1 {
					return tag + "." + c
				}
			}
			return tag + "." + classes[0]
		}
	}

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
