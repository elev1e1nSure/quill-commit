package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"quill-commit/credentials"
	"quill-commit/releasenotes"
)

func getAPIKeyFallback() string {
	if env := os.Getenv("QUILL_API_KEY"); env != "" {
		return env
	}
	if env := os.Getenv("OPENROUTER_API_KEY"); env != "" {
		return env
	}
	stored, err := credentials.Load()
	if err != nil {
		return ""
	}
	return stored
}

func main() {
	from := flag.String("from", "", "from ref (tag or commit hash)")
	to := flag.String("to", "HEAD", "to ref (tag or commit hash)")
	apiKey := flag.String("api-key", getAPIKeyFallback(), "OpenRouter API key")
	model := flag.String("model", "deepseek/deepseek-v4-flash", "AI model name")
	initial := flag.Bool("initial", false, "initial release — describe the software itself")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: API key required — set --api-key flag, set QUILL_API_KEY/OPENROUTER_API_KEY env, or run main tool with key to save it")
		os.Exit(1)
	}
	if *from == "" {
		fmt.Fprintln(os.Stderr, "error: --from is required")
		os.Exit(1)
	}

	notes, err := releasenotes.Generate(context.Background(), *from, *to, *apiKey, *model, *initial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(notes)
}
