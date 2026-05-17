package engine

import (
	"html"
	"math"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// maxCellSpan caps colspan/rowspan attribute values to defend against
// pathological inputs that would otherwise produce huge expanded rows.
const maxCellSpan = 100

// TableKind classifies whether a <table> carries semantic tabular data or is
// being used purely for visual layout. Layout tables get flattened during
// preprocess; data tables are preserved (and optionally extracted as
// structured TableRef values when Options.WithTables is set).
type TableKind int

const (
	TableData TableKind = iota
	TableLayout
)

// TableRef is a structured, host-agnostic representation of an HTML <table>
// classified as data. Surfaced on Result.Tables when the caller opts in via
// Options.WithTables.
//
// Cells with colspan/rowspan expand into a regular grid; rows in
// TableRef.Rows have the same column count as headers when present.
// Cell-internal HTML (<a>, <strong>, etc.) is flattened to text; HTML
// entities are decoded; internal whitespace is collapsed.
type TableRef struct {
	Caption string     `json:"caption,omitempty"`
	Headers []string   `json:"headers,omitempty"`
	Rows    [][]string `json:"rows"`
}

// classifyTable applies a host-agnostic heuristic to decide whether n is a
// data or layout table.
//
//	semantic markers → TableData:
//	  - any <th> descendant
//	  - any <caption> descendant
//	  - any <thead> descendant
//
//	otherwise count <tr> rows and per-row cell counts; rows ≥ 2 AND
//	avgCols ≥ 2 AND stddev(cols) < 0.5 * avgCols → TableData. Else TableLayout.
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
		// Skip nested <table> subtrees — only count rows of THIS table.
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

// unwrapLayoutTables walks the document, classifies every (outer) <table>, and
// replaces TableLayout tables with the visible text of their cells wrapped in
// <div> blocks. Data tables are left untouched.
//
// Nested tables are not processed independently: when an outer table is
// classified as layout and unwrapped, its nested children disappear with it.
// When an outer table is data, nested children are left in place.
//
// If nothing is unwrapped the original string is returned unchanged.
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
		cellTexts := collectCellTexts(t)
		// Build a sequence of <div> nodes (one per non-empty cell) and insert
		// them in place of the table.
		var divs []*xhtml.Node
		for _, txt := range cellTexts {
			if txt == "" {
				continue
			}
			div := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
			div.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: txt})
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

// ExtractTables walks htmlStr and returns one TableRef per <table> classified
// as data. baseURL is reserved for future use (e.g. resolving in-cell hrefs);
// V1 only extracts text.
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

// collectOuterTables returns every <table> element that has no <table>
// ancestor — i.e. the outermost tables only.
func collectOuterTables(root *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			out = append(out, n)
			return // Don't descend — nested tables are handled when their outer is processed.
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return out
}

// collectCellTexts returns the cleaned text of every <td>/<th> descendant of
// the table, in document order. Empty cells produce empty strings (caller
// decides whether to keep them).
func collectCellTexts(table *xhtml.Node) []string {
	var out []string
	var visit func(n *xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n != table && n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			return
		}
		if n.Type == xhtml.ElementNode && (n.DataAtom == atom.Td || n.DataAtom == atom.Th) {
			out = append(out, cellText(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(table)
	return out
}

// buildTableRef extracts caption + header + rows from a TableData node.
func buildTableRef(table *xhtml.Node) TableRef {
	ref := TableRef{Rows: [][]string{}}

	// Caption: first <caption> descendant.
	if cap := findFirstByAtom(table, atom.Caption); cap != nil {
		ref.Caption = cellText(cap)
	}

	// Walk rows in document order. The first <tr> that contains any <th>
	// cells becomes the headers row; all subsequent <tr> contribute data rows.
	// If no <th>-bearing row exists, every <tr> is a data row.
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

// pendingSpan tracks a cell whose rowspan extends into rows below the one
// that physically declared it. `remaining` is the number of additional rows
// that still need to receive `text` at this column index.
type pendingSpan struct {
	text      string
	remaining int
}

// childCellNodes returns the direct-child <td>/<th> nodes of a <tr> in
// document order.
func childCellNodes(tr *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
			out = append(out, c)
		}
	}
	return out
}

// cellSpan parses colspan/rowspan attributes on n. Returns (cols, rows) with
// defaults of 1 each and a maxCellSpan cap per axis to defend against
// pathological inputs. Non-numeric, zero, or negative values fall back to 1.
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

// expandRow places cells from tr into a logical-column-indexed slice,
// honouring colspan + the carry-over from prior rows' rowspans. After
// returning, pending is updated for the NEXT row.
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
		// Fill any columns claimed by a still-pending rowspan before placing
		// the next physical cell.
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
	// After all physical cells: fill any trailing pending columns (some rows
	// end before later spans).
	consumePending()
	return row
}

// cellText walks descendant text nodes, joins them with single spaces,
// decodes HTML entities, trims, and collapses internal whitespace runs.
func cellText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(node *xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node == nil {
			return
		}
		if node.Type == xhtml.ElementNode {
			switch node.DataAtom {
			case atom.Script, atom.Style:
				return
			}
		}
		if node.Type == xhtml.TextNode {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return collapseWhitespace(html.UnescapeString(b.String()))
}
