// Command commenteraser removes non-directive Go comments in place and gofmts
// the result. It preserves compiler/tool directives (//go:build, //go:generate,
// //go:embed, //nolint:..., //line, //export, //extern) and the cgo `import "C"`
// preamble, since stripping those would change the build. Skips generated files.
//
// Used by `./dev strip-comments`. Pass the Go files to process as arguments.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
)

var generatedRE = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// isDirective reports whether a `//` line comment body (the text after "//") is a
// compiler/tool directive that must be kept. Mirrors go/ast's rule: a leading
// "line "/"extern "/"export ", or "//word:arg" with a lowercase-alnum word.
func isDirective(text string) bool {
	if strings.HasPrefix(text, "line ") || strings.HasPrefix(text, "extern ") || strings.HasPrefix(text, "export ") {
		return true
	}
	// Legacy build constraint (`// +build tags`) — keep it even without //go:build.
	if strings.HasPrefix(strings.TrimSpace(text), "+build") {
		return true
	}
	colon := strings.Index(text, ":")
	if colon <= 0 || colon+1 >= len(text) {
		return false
	}
	for i := 0; i < colon; i++ {
		b := text[i]
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9') {
			return false
		}
	}
	return true
}

func stripFile(path string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if generatedRE.Match(src) {
		return false, nil // never rewrite generated files
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return false, err
	}

	// Preserve the cgo preamble (block comment attached to `import "C"`).
	preserve := map[*ast.Comment]bool{}
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"C"` && imp.Doc != nil {
			for _, c := range imp.Doc.List {
				preserve[c] = true
			}
		}
	}

	type span struct{ start, end int }
	var spans []span
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if preserve[c] {
				continue
			}
			if strings.HasPrefix(c.Text, "//") && isDirective(c.Text[2:]) {
				continue
			}
			start := fset.Position(c.Pos()).Offset
			end := fset.Position(c.End()).Offset
			// When the comment is alone on its line, swallow the whole line
			// (leading indent + trailing newline) so no blank line is left behind.
			ls := start
			for ls > 0 && src[ls-1] != '\n' {
				ls--
			}
			if strings.TrimSpace(string(src[ls:start])) == "" {
				start = ls
				if end < len(src) && src[end] == '\n' {
					end++
				}
			}
			spans = append(spans, span{start, end})
		}
	}
	if len(spans) == 0 {
		return false, nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var out bytes.Buffer
	prev := 0
	for _, s := range spans {
		if s.start < prev {
			continue // overlap guard
		}
		out.Write(src[prev:s.start])
		prev = s.end
	}
	out.Write(src[prev:])

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return false, fmt.Errorf("gofmt after strip: %w", err)
	}
	if bytes.Equal(formatted, src) {
		return false, nil
	}
	return true, os.WriteFile(path, formatted, 0o644)
}

func main() {
	changed, failed := 0, 0
	for _, p := range os.Args[1:] {
		ok, err := stripFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			failed++
			continue
		}
		if ok {
			changed++
		}
	}
	fmt.Printf("commenteraser: %d file(s) stripped, %d skipped/failed\n", changed, failed)
}
