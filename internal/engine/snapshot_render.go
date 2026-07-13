package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (n *SnapshotNode) ToCompact() string {
	var lines []string
	n.toCompactLines(&lines, 0)
	return strings.Join(lines, "\n")
}

func (n *SnapshotNode) toCompactLines(lines *[]string, indent int) {
	prefix := strings.Repeat("  ", indent)

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

func truncateToTokens(root *SnapshotNode, maxTokens int) *SnapshotNode {
	maxChars := maxTokens * 4

	data, _ := json.Marshal(root)
	if len(data) <= maxChars {
		return root
	}

	result := *root
	result.Children = truncateChildren(root.Children, maxChars, len(data))
	return &result
}

func truncateChildren(children []SnapshotNode, maxChars, currentSize int) []SnapshotNode {
	if currentSize <= maxChars || len(children) == 0 {
		return children
	}

	result := make([]SnapshotNode, 0, len(children))
	for i, child := range children {
		childData, _ := json.Marshal(child)
		childSize := len(childData)

		if currentSize-childSize > maxChars && i > len(children)/2 {
			currentSize -= childSize
			continue
		}

		truncated := child
		truncated.Children = truncateChildren(child.Children, maxChars/2, childSize)
		result = append(result, truncated)
	}

	return result
}
