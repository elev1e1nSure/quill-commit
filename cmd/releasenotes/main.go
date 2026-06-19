package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"quill-commit/releasenotes"
)

func main() {
	from := flag.String("from", "", "from ref (tag or commit hash)")
	to := flag.String("to", "HEAD", "to ref (tag or commit hash)")
	apiKey := flag.String("api-key", os.Getenv("OPENROUTER_API_KEY"), "OpenRouter API key")
	model := flag.String("model", "deepseek/deepseek-v4-flash", "AI model name")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: API key required — set --api-key flag or OPENROUTER_API_KEY env")
		os.Exit(1)
	}
	if *from == "" {
		fmt.Fprintln(os.Stderr, "error: --from is required")
		os.Exit(1)
	}

	notes, err := releasenotes.Generate(context.Background(), *from, *to, *apiKey, *model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(notes)
}
