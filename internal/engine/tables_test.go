package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// firstTable parses an HTML snippet and returns its first <table> element.
func firstTable(t *testing.T, snippet string) *xhtml.Node {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(snippet))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var visit func(n *xhtml.Node) *xhtml.Node
	visit = func(n *xhtml.Node) *xhtml.Node {
		if n == nil {
			return nil
		}
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Table {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if got := visit(c); got != nil {
				return got
			}
		}
		return nil
	}
	tab := visit(doc)
	if tab == nil {
		t.Fatalf("no <table> found in snippet")
	}
	return tab
}

func TestClassifyTable_HasTH(t *testing.T) {
	got := classifyTable(firstTable(t, `<table><tr><th>A</th><td>1</td></tr></table>`))
	if got != TableData {
		t.Fatalf("got %v, want TableData", got)
	}
}

func TestClassifyTable_HasThead(t *testing.T) {
	html := `<table><thead><tr><td>H</td></tr></thead><tbody><tr><td>x</td></tr></tbody></table>`
	if got := classifyTable(firstTable(t, html)); got != TableData {
		t.Fatalf("got %v, want TableData", got)
	}
}

func TestClassifyTable_HasCaption(t *testing.T) {
	html := `<table><caption>Cap</caption><tr><td>x</td></tr></table>`
	if got := classifyTable(firstTable(t, html)); got != TableData {
		t.Fatalf("got %v, want TableData", got)
	}
}

func TestClassifyTable_RowsAndColsHeuristic(t *testing.T) {
	html := `<table>
        <tr><td>a</td><td>b</td><td>c</td></tr>
        <tr><td>d</td><td>e</td><td>f</td></tr>
        <tr><td>g</td><td>h</td><td>i</td></tr>
    </table>`
	if got := classifyTable(firstTable(t, html)); got != TableData {
		t.Fatalf("got %v, want TableData", got)
	}
}

func TestClassifyTable_LayoutNoTHUnevenCols(t *testing.T) {
	html := `<table>
        <tr><td>a</td><td>b</td><td>c</td></tr>
        <tr><td>only one</td></tr>
    </table>`
	if got := classifyTable(firstTable(t, html)); got != TableLayout {
		t.Fatalf("got %v, want TableLayout", got)
	}
}

func TestClassifyTable_LayoutSingleRowSingleCol(t *testing.T) {
	html := `<table><tr><td>just text</td></tr></table>`
	if got := classifyTable(firstTable(t, html)); got != TableLayout {
		t.Fatalf("got %v, want TableLayout", got)
	}
}

func TestUnwrapLayoutTables_FlattensLayoutTable(t *testing.T) {
	in := `<html><body><div><table>
        <tr><td>alpha</td><td>beta</td><td>gamma</td></tr>
        <tr><td>only one</td></tr>
    </table></div></body></html>`
	out := unwrapLayoutTables(in)
	if strings.Contains(out, "<table") {
		t.Fatalf("expected layout table removed, got: %s", out)
	}
	for _, needle := range []string{"alpha", "beta", "gamma", "only one"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected cell text %q to survive, got: %s", needle, out)
		}
	}
	if !strings.Contains(out, "<div>alpha</div>") {
		t.Fatalf("expected <div>alpha</div>, got: %s", out)
	}
}

func TestUnwrapLayoutTables_LeavesDataTableIntact(t *testing.T) {
	in := `<html><body><table><thead><tr><th>A</th><th>B</th></tr></thead>` +
		`<tbody><tr><td>1</td><td>2</td></tr><tr><td>3</td><td>4</td></tr></tbody></table></body></html>`
	out := unwrapLayoutTables(in)
	if !strings.Contains(out, "<table>") || !strings.Contains(out, "<thead>") {
		t.Fatalf("expected data table preserved, got: %s", out)
	}
}

func TestExtractTables_BasicHeadersAndRows(t *testing.T) {
	html := `<html><body><table>
        <tr><th>Name</th><th>Score</th></tr>
        <tr><td>Alice</td><td>10</td></tr>
        <tr><td>Bob</td><td>20</td></tr>
    </table></body></html>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "Name" || tbl.Headers[1] != "Score" {
		t.Fatalf("headers: %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows: %v", tbl.Rows)
	}
	if tbl.Rows[0][0] != "Alice" || tbl.Rows[1][1] != "20" {
		t.Fatalf("rows: %v", tbl.Rows)
	}
}

func TestExtractTables_PreservesCaption(t *testing.T) {
	html := `<table><caption>My Caption</caption>
        <tr><th>A</th></tr><tr><td>x</td></tr></table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 || tables[0].Caption != "My Caption" {
		t.Fatalf("caption: %#v", tables)
	}
}

func TestExtractTables_DecodesEntities(t *testing.T) {
	html := `<table><tr><th>Symbol</th></tr><tr><td>AT&amp;T</td></tr><tr><td>it&#39;s</td></tr></table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d", len(tables))
	}
	if tables[0].Rows[0][0] != "AT&T" {
		t.Fatalf("row0: %q", tables[0].Rows[0][0])
	}
	if tables[0].Rows[1][0] != "it's" {
		t.Fatalf("row1: %q", tables[0].Rows[1][0])
	}
}

func TestExtractTables_TrimsCellWhitespace(t *testing.T) {
	html := "<table><tr><th>X</th></tr><tr><td>  hello\n\n   world  </td></tr></table>"
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d", len(tables))
	}
	got := tables[0].Rows[0][0]
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestExtractTables_SkipsLayoutTables(t *testing.T) {
	html := `<html><body>
        <table>
            <tr><td>nav</td><td>logo</td><td>menu</td></tr>
            <tr><td>solo</td></tr>
        </table>
        <table>
            <thead><tr><th>K</th><th>V</th></tr></thead>
            <tbody><tr><td>a</td><td>1</td></tr><tr><td>b</td><td>2</td></tr></tbody>
        </table>
    </body></html>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1 (data only)", len(tables))
	}
	if len(tables[0].Headers) != 2 {
		t.Fatalf("headers: %v", tables[0].Headers)
	}
}

func TestExtractTables_ColspanExpandsCell(t *testing.T) {
	html := `<table>
        <tr><th>A</th><th>B</th></tr>
        <tr><td colspan="2">X</td></tr>
        <tr><td>A</td><td>B</td></tr>
    </table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Headers) != 2 {
		t.Fatalf("headers width: %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows: %v", tbl.Rows)
	}
	if len(tbl.Rows[0]) != 2 || tbl.Rows[0][0] != "X" || tbl.Rows[0][1] != "X" {
		t.Fatalf("row0 colspan expansion: %v", tbl.Rows[0])
	}
	if len(tbl.Rows[1]) != 2 || tbl.Rows[1][0] != "A" || tbl.Rows[1][1] != "B" {
		t.Fatalf("row1 normal: %v", tbl.Rows[1])
	}
}

func TestExtractTables_RowspanExpandsCell(t *testing.T) {
	html := `<table>
        <tr><th>A</th><th>B</th></tr>
        <tr><td rowspan="2">X</td><td>B</td></tr>
        <tr><td>D</td></tr>
    </table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows: %v", tbl.Rows)
	}
	if len(tbl.Rows[0]) != 2 || tbl.Rows[0][0] != "X" || tbl.Rows[0][1] != "B" {
		t.Fatalf("row0: %v", tbl.Rows[0])
	}
	if len(tbl.Rows[1]) != 2 || tbl.Rows[1][0] != "X" || tbl.Rows[1][1] != "D" {
		t.Fatalf("row1 carried rowspan: %v", tbl.Rows[1])
	}
}

func TestExtractTables_MixedColspanRowspan(t *testing.T) {
	// Infobox-style: header spans 2 cols; first data row has a label that
	// rowspans into the next row.
	html := `<table>
        <tr><th colspan="2">Title</th></tr>
        <tr><td rowspan="2">Label</td><td>val1</td></tr>
        <tr><td>val2</td></tr>
    </table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "Title" || tbl.Headers[1] != "Title" {
		t.Fatalf("headers: %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows: %v", tbl.Rows)
	}
	if len(tbl.Rows[0]) != 2 || tbl.Rows[0][0] != "Label" || tbl.Rows[0][1] != "val1" {
		t.Fatalf("row0: %v", tbl.Rows[0])
	}
	if len(tbl.Rows[1]) != 2 || tbl.Rows[1][0] != "Label" || tbl.Rows[1][1] != "val2" {
		t.Fatalf("row1: %v", tbl.Rows[1])
	}
}

func TestExtractTables_CapsLargeSpans(t *testing.T) {
	html := `<table>
        <tr><th>A</th></tr>
        <tr><td colspan="9999">huge</td></tr>
    </table>`
	tables := ExtractTables(html, "")
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Rows) != 1 {
		t.Fatalf("rows: %v", tbl.Rows)
	}
	if len(tbl.Rows[0]) != 100 {
		t.Fatalf("expected cap at 100, got %d", len(tbl.Rows[0]))
	}
	for i, c := range tbl.Rows[0] {
		if c != "huge" {
			t.Fatalf("cell %d: %q", i, c)
		}
	}
}

func TestExtract_TablesFlagOn(t *testing.T) {
	body := `<html><body><article>
        <h1>Stats</h1>
        <p>Some prose here to give readability something substantial to chew on.
        Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod
        tempor incididunt ut labore et dolore magna aliqua.</p>
        <table>
            <tr><td>layout-a</td><td>layout-b</td><td>layout-c</td></tr>
            <tr><td>solo cell</td></tr>
        </table>
        <table>
            <thead><tr><th>Key</th><th>Value</th></tr></thead>
            <tbody><tr><td>alpha</td><td>1</td></tr><tr><td>beta</td><td>2</td></tr></tbody>
        </table>
    </article></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{WithTables: true})
	if len(res.Tables) != 1 {
		t.Fatalf("got %d tables, want 1; tables=%+v", len(res.Tables), res.Tables)
	}
	tbl := res.Tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "Key" {
		t.Fatalf("headers: %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 || tbl.Rows[0][0] != "alpha" {
		t.Fatalf("rows: %v", tbl.Rows)
	}
}

func TestExtract_TablesFlagOffDefault(t *testing.T) {
	body := `<html><body><article><h1>X</h1><p>hello</p>
        <table><thead><tr><th>a</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table>
    </article></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res := FromURLWithOptions(srv.URL, Options{})
	if res.Tables != nil {
		t.Fatalf("expected nil Tables when WithTables=false, got %+v", res.Tables)
	}
}
