package engine

import (
	"html"
	"math"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const maxCellSpan = 100

type TableKind int

const (
	TableData TableKind = iota
	TableLayout
)

type TableRef struct {
	Caption string     `json:"caption,omitempty"`
	Headers []string   `json:"headers,omitempty"`
	Rows    [][]string `json:"rows"`
}

func classifyTable(n *xhtml.Node) TableKind {
	if n == nil {
		return TableLayout
	}

	if hasDescendant(n, atom.Th) || hasDescendant(n, atom.Caption) || hasDescendant(n, atom.Thead) {
		return TableData
	}

	var colsPerRow []int
	var visit func(node *xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node == nil {
			return
		}
		if node != n && node.Type == xhtml.ElementNode && node.DataAtom == atom.Table {
			return
		}
		if node.Type == xhtml.ElementNode && node.DataAtom == atom.Tr {
			cells := 0
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
					cells++
				}
			}
			if cells > 0 {
				colsPerRow = append(colsPerRow, cells)
			}
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)

	if len(colsPerRow) < 2 {
		return TableLayout
	}
	sum := 0
	for _, c := range colsPerRow {
		sum += c
	}
	avg := float64(sum) / float64(len(colsPerRow))
	if avg < 2 {
		return TableLayout
	}
	var variance float64
	for _, c := range colsPerRow {
		d := float64(c) - avg
		variance += d * d
	}
	variance /= float64(len(colsPerRow))
	stddev := math.Sqrt(variance)
	if stddev < 0.5*avg {
		return TableData
	}
	return TableLayout
}

func unwrapLayoutTables(htmlStr string) string {
	if htmlStr == "" {
		return htmlStr
	}
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	tables := collectOuterTables(doc)
	if len(tables) == 0 {
		return htmlStr
	}

	unwrapped := 0
	for _, t := range tables {
		if classifyTable(t) != TableLayout {
			continue
		}
		parent := t.Parent
		if parent == nil {
			continue
		}
		var divs []*xhtml.Node
		for _, cell := range collectOwnCells(t) {
			div := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
			for c := cell.FirstChild; c != nil; {
				next := c.NextSibling
				cell.RemoveChild(c)
				div.AppendChild(c)
				c = next
			}
			if div.FirstChild == nil {
				continue
			}
			divs = append(divs, div)
		}
		next := t.NextSibling
		parent.RemoveChild(t)
		for _, d := range divs {
			if next != nil {
				parent.InsertBefore(d, next)
			} else {
				parent.AppendChild(d)
			}
		}
		unwrapped++
	}

	if unwrapped == 0 {
		return htmlStr
	}
	return renderNode(doc)
}

func ExtractTables(htmlStr string, _ string) []TableRef {
	if htmlStr == "" {
		return nil
	}
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	tables := collectOuterTables(doc)
	if len(tables) == 0 {
		return nil
	}

	var out []TableRef
	for _, t := range tables {
		if classifyTable(t) != TableData {
			continue
		}
		out = append(out, buildTableRef(t))
	}
	return out
}

func collectOuterTables(root *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return out
}

func collectOwnCells(table *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n != table && n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			return
		}
		if n.Type == xhtml.ElementNode && (n.DataAtom == atom.Td || n.DataAtom == atom.Th) {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(table)
	return out
}

func buildTableRef(table *xhtml.Node) TableRef {
	ref := TableRef{Rows: [][]string{}}

	if cap := findFirstByAtom(table, atom.Caption); cap != nil {
		ref.Caption = cellText(cap)
	}

	var rows []*xhtml.Node
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n != table && n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			return
		}
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Tr {
			rows = append(rows, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(table)

	pending := map[int]pendingSpan{}
	headerTaken := false
	for _, tr := range rows {
		if !headerTaken && rowHasTH(tr) {
			ref.Headers = expandRow(tr, pending)
			headerTaken = true
			continue
		}
		cells := expandRow(tr, pending)
		if len(cells) == 0 {
			continue
		}
		ref.Rows = append(ref.Rows, cells)
	}

	return ref
}

func rowHasTH(tr *xhtml.Node) bool {
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && c.DataAtom == atom.Th {
			return true
		}
	}
	return false
}

type pendingSpan struct {
	text      string
	remaining int
}

func childCellNodes(tr *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
			out = append(out, c)
		}
	}
	return out
}

func cellSpan(n *xhtml.Node) (cols, rows int) {
	cols, rows = 1, 1
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "colspan":
			if v, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil && v >= 1 {
				cols = v
			}
		case "rowspan":
			if v, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil && v >= 1 {
				rows = v
			}
		}
	}
	if cols > maxCellSpan {
		cols = maxCellSpan
	}
	if rows > maxCellSpan {
		rows = maxCellSpan
	}
	return
}

func expandRow(tr *xhtml.Node, pending map[int]pendingSpan) []string {
	var row []string
	col := 0
	cells := childCellNodes(tr)

	consumePending := func() {
		for {
			ps, ok := pending[col]
			if !ok {
				return
			}
			for len(row) <= col {
				row = append(row, "")
			}
			row[col] = ps.text
			ps.remaining--
			if ps.remaining <= 0 {
				delete(pending, col)
			} else {
				pending[col] = ps
			}
			col++
		}
	}

	for _, cell := range cells {
		consumePending()

		text := cellText(cell)
		cols, rows := cellSpan(cell)
		for c := 0; c < cols; c++ {
			for len(row) <= col {
				row = append(row, "")
			}
			row[col] = text
			if rows > 1 {
				pending[col] = pendingSpan{text: text, remaining: rows - 1}
			}
			col++
		}
	}
	consumePending()
	return row
}

func cellText(n *xhtml.Node) string {
	text := nodeTextOpts(n, textOptions{spaceJoin: true, skipScriptStyle: true})
	return collapseWhitespace(html.UnescapeString(text))
}
