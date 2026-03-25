package main

import (
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

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", result.Error)
		os.Exit(1)
	}

	fmt.Printf("Title:   %s\n", result.Title)
	fmt.Printf("Profile: %s\n", result.Profile)
	fmt.Printf("Quality: %.0f%%\n\n", result.Quality)
	fmt.Println(result.Content)
}
