// seaportal extracts clean Markdown from URLs with SPA detection
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/seaportal/pkg/portal"
)

var version = "dev"

func main() {
	noDedupe := flag.Bool("no-dedupe", false, "Disable deduplication (enabled by default)")
	fast := flag.Bool("fast", false, "Fast mode: bail early if browser is needed")
	jsonOut := flag.Bool("json", false, "Output JSON instead of Markdown")
	showVersion := flag.Bool("version", false, "Show version")
	flag.BoolVar(showVersion, "v", false, "Show version")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "SeaPortal - Extract clean Markdown from URLs with SPA detection")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  seaportal [options] <url>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("seaportal %s\n", version)
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	targetURL := args[0]
	dedupe := !*noDedupe

	opts := portal.Options{Dedupe: dedupe, FastMode: *fast}
	result := portal.FromURLWithOptions(targetURL, opts)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	var output strings.Builder
	output.WriteString("---\n")
	fmt.Fprintf(&output, "title: %q\n", result.Title)
	fmt.Fprintf(&output, "url: %s\n", result.URL)
	fmt.Fprintf(&output, "byline: %q\n", result.Byline)
	fmt.Fprintf(&output, "excerpt: %q\n", result.Excerpt)
	fmt.Fprintf(&output, "sitename: %q\n", result.SiteName)
	fmt.Fprintf(&output, "length: %d\n", result.Length)
	fmt.Fprintf(&output, "confidence: %d\n", result.Confidence)
	fmt.Fprintf(&output, "isSpa: %v\n", result.IsSPA)
	if len(result.SPASignals) > 0 {
		fmt.Fprintf(&output, "spaSignals: %v\n", result.SPASignals)
	}
	fmt.Fprintf(&output, "pageClass: %s\n", result.Profile.Class)
	fmt.Fprintf(&output, "outcome: %s\n", result.Profile.Outcome)
	fmt.Fprintf(&output, "trustworthy: %v\n", result.Profile.Trustworthy)
	if len(result.Profile.Reasons) > 0 {
		fmt.Fprintf(&output, "classReasons: %v\n", result.Profile.Reasons)
	}
	fmt.Fprintf(&output, "headings: %d\n", result.HeadingCount)
	fmt.Fprintf(&output, "links: %d\n", result.LinkCount)
	fmt.Fprintf(&output, "paragraphs: %d\n", result.ParagraphCount)
	if result.DedupeApplied {
		fmt.Fprintf(&output, "dedupeApplied: %v\n", result.DedupeApplied)
		fmt.Fprintf(&output, "duplicatesRemoved: %d\n", result.DuplicatesRemoved)
		if len(result.DuplicateSignals) > 0 {
			fmt.Fprintf(&output, "duplicateSignals: %v\n", result.DuplicateSignals)
		}
	}
	fmt.Fprintf(&output, "validationOk: %v\n", result.Validation.IsValid)
	fmt.Fprintf(&output, "needsBrowser: %v\n", result.Validation.NeedsBrowser)
	fmt.Fprintf(&output, "validationConfidence: %.2f\n", result.Validation.Confidence)
	if len(result.Validation.Issues) > 0 {
		fmt.Fprintf(&output, "validationIssues: %v\n", result.Validation.Issues)
	}
	output.WriteString("---\n\n")
	output.WriteString(result.Content)

	domain := strings.ReplaceAll(strings.Split(targetURL, "//")[1], "/", "_")
	if idx := strings.Index(domain, "/"); idx > 0 {
		domain = domain[:idx]
	}
	domain = strings.ReplaceAll(domain, ":", "_")
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join("renders", "seaportal", fmt.Sprintf("%s_%s.md", domain, timestamp))

	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create directory: %v\n", err)
	}
	if err := os.WriteFile(filename, []byte(output.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save markdown: %v\n", err)
	}

	jsonFile := filepath.Join("renders", "seaportal", fmt.Sprintf("%s_%s.json", domain, timestamp))
	jsonData, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save JSON: %v\n", err)
	}

	fmt.Printf("Saved: %s (%d bytes, %dms, confidence: %d%%)\n", filename, result.Length, result.TimeMs, result.Confidence)
	fmt.Printf("📋 Classification: %s\n", result.Profile.String())
	if len(result.Profile.Reasons) > 0 {
		fmt.Printf("   Reasons: %v\n", result.Profile.Reasons)
	}
	if result.IsSPA {
		fmt.Printf("⚠️  SPA detected: %v\n", result.SPASignals)
	}
	fmt.Println("\n--- Content ---")
	fmt.Println(output.String())
}
