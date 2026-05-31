package engine

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestResultToTEIXML_BasicSkeleton(t *testing.T) {
	r := Result{
		Title:   "Foo",
		Byline:  "Alice",
		URL:     "https://x.com",
		Content: "Hello world.",
	}
	out, err := ResultToTEIXML(r)
	if err != nil {
		t.Fatalf("ResultToTEIXML: %v", err)
	}
	s := string(out)
	for _, sub := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`xmlns="http://www.tei-c.org/ns/1.0"`,
		`<title>Foo</title>`,
		`<author>Alice</author>`,
		`<ref target="https://x.com">https://x.com</ref>`,
	} {
		if !strings.Contains(s, sub) {
			t.Errorf("output missing %q\n---\n%s", sub, s)
		}
	}
}

func TestMarkdownToTEI_Headings(t *testing.T) {
	nodes := markdownToTEI("# A\n\n## B")
	var heads []teiNode
	for _, n := range nodes {
		if n.XMLName.Local == "head" {
			heads = append(heads, n)
		}
	}
	if len(heads) != 2 {
		t.Fatalf("want 2 head elements, got %d (%v)", len(heads), nodes)
	}
	if heads[0].Type != "h1" || heads[1].Type != "h2" {
		t.Errorf("want types h1/h2, got %s/%s", heads[0].Type, heads[1].Type)
	}
	if heads[0].Chardata != "A" || heads[1].Chardata != "B" {
		t.Errorf("head text: %q / %q", heads[0].Chardata, heads[1].Chardata)
	}
}

func TestMarkdownToTEI_Lists(t *testing.T) {
	nodes := markdownToTEI("- a\n- b")
	if len(nodes) != 1 {
		t.Fatalf("want 1 list node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.XMLName.Local != "list" || n.Type != "unordered" {
		t.Fatalf("want <list type=unordered>, got %+v", n)
	}
	if len(n.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(n.Items))
	}
	if n.Items[0].XMLName.Local != "item" || n.Items[0].Chardata != "a" {
		t.Errorf("item[0]: %+v", n.Items[0])
	}
}

func TestMarkdownToTEI_OrderedList(t *testing.T) {
	nodes := markdownToTEI("1. a\n2. b")
	if len(nodes) != 1 {
		t.Fatalf("want 1 list node, got %d", len(nodes))
	}
	if nodes[0].Type != "ordered" {
		t.Fatalf("want ordered, got %q", nodes[0].Type)
	}
	if len(nodes[0].Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(nodes[0].Items))
	}
}

func TestMarkdownToTEI_CodeFences(t *testing.T) {
	md := "```go\nfunc()\n```"
	nodes := markdownToTEI(md)
	if len(nodes) != 1 {
		t.Fatalf("want 1 code node, got %d: %+v", len(nodes), nodes)
	}
	n := nodes[0]
	if n.XMLName.Local != "code" {
		t.Fatalf("want <code>, got %s", n.XMLName.Local)
	}
	if n.Lang != "go" {
		t.Errorf("want lang=go, got %q", n.Lang)
	}
	if !strings.Contains(n.Chardata, "func()") {
		t.Errorf("want body to contain func(), got %q", n.Chardata)
	}
}

func TestMarkdownToTEI_Paragraphs(t *testing.T) {
	nodes := markdownToTEI("Line one.\nLine two.\n\nNext para.")
	var ps []teiNode
	for _, n := range nodes {
		if n.XMLName.Local == "p" {
			ps = append(ps, n)
		}
	}
	if len(ps) != 2 {
		t.Fatalf("want 2 <p>, got %d (%+v)", len(ps), nodes)
	}
	if !strings.Contains(ps[0].Chardata, "Line one") || !strings.Contains(ps[0].Chardata, "Line two") {
		t.Errorf("para 1 should join two lines, got %q", ps[0].Chardata)
	}
	if !strings.Contains(ps[1].Chardata, "Next para") {
		t.Errorf("para 2 wrong: %q", ps[1].Chardata)
	}
}

func TestResultToTEIXML_WellFormed(t *testing.T) {
	r := Result{
		Title:         "Foo",
		Byline:        "Alice",
		URL:           "https://example.com",
		PublishedDate: "2024-01-01",
		Language:      "en",
		Content:       "# Heading\n\nBody paragraph.\n\n- one\n- two\n\n```py\nprint(1)\n```\n",
	}
	out, err := ResultToTEIXML(r)
	if err != nil {
		t.Fatalf("ResultToTEIXML: %v", err)
	}
	var generic interface{}
	if err := xml.Unmarshal(out, &generic); err != nil {
		t.Fatalf("round-trip Unmarshal: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `<language ident="en"`) {
		t.Errorf("missing language element: %s", out)
	}
	if !strings.Contains(string(out), `<date>2024-01-01</date>`) {
		t.Errorf("missing date element: %s", out)
	}
}

func TestResultToTEIXML_EmptyBody(t *testing.T) {
	r := Result{Title: "T", URL: "https://x.com"}
	out, err := ResultToTEIXML(r)
	if err != nil {
		t.Fatalf("ResultToTEIXML: %v", err)
	}
	if !strings.Contains(string(out), "<body>") {
		t.Errorf("missing <body>: %s", out)
	}
}
