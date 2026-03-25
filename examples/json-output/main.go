package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pinchtab/seaportal/pkg/portal"
)

func main() {
	url := "https://example.com"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	opts := portal.Options{Dedupe: true}
	result := portal.FromURLWithOptions(url, opts)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
