// Command seabench is the SeaPortal benchmark / evaluation harness.
//
// The first subcommand is `eval`: run the corpus at tests/eval/corpus.yaml
// through four in-process extractors (seaportal, go-readability standalone,
// html-to-markdown standalone, strip-tags baseline) and emit a Markdown
// report with per-extractor precision / recall / F1 plus machine-independent
// time ratios relative to the strip-tags baseline.
//
// All extractors run in-process — no subprocesses, no network, no LLM. The
// scoring signal comes entirely from must_include / must_exclude substring
// lists declared in the corpus YAML.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "eval":
		runEval(os.Args[2:])
	case "stress":
		runStress(os.Args[2:])
	case "classify":
		runClassify(os.Args[2:])
	case "tokens":
		runTokens(os.Args[2:])
	case "cachebench":
		runCacheBench(os.Args[2:])
	case "diff":
		runDiff(os.Args[2:])
	case "selftest":
		runSelftest(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "seabench — SeaPortal benchmark harness")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  seabench eval [--corpus PATH] [--report-dir DIR] [--baseline]")
	fmt.Fprintln(os.Stderr, "  seabench stress [--preset quick|small|medium|large] [--baseline FILE] [--output DIR] [--fixture PATH]")
	fmt.Fprintln(os.Stderr, "  seabench classify [--corpus FILE] [--output DIR]")
	fmt.Fprintln(os.Stderr, "  seabench tokens [--corpus FILE] [--output DIR]")
	fmt.Fprintln(os.Stderr, "  seabench cachebench [--n 200] [--hot-ratio 0.8] [--hot N] [--cold N] [--seed 42] [--output DIR]")
	fmt.Fprintln(os.Stderr, "  seabench diff [--corpus FILE] [--output DIR] [--snippet-chars 400]")
	fmt.Fprintln(os.Stderr, "  seabench selftest [--input FILE.jsonl] [--group FILE.md] [--output DIR]")
	fmt.Fprintln(os.Stderr, "  seabench help")
}
