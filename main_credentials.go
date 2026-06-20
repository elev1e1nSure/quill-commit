package main

import (
	"fmt"
	"os"

	"quill-commit/credentials"
)

// CredentialResolver resolves the OpenRouter API key from flag → env → file.
type CredentialResolver struct {
	FlagValue string
}

// Resolve returns the API key to use and whether it should be persisted.
// The value comes from (in order): --api-key flag, QUILL_API_KEY env var,
// credentials file. shouldSave is true only when the key was provided via flag.
func (r *CredentialResolver) Resolve() (apiKey string, shouldSave bool, err error) {
	if r.FlagValue != "" {
		return r.FlagValue, true, nil
	}
	if env := os.Getenv("QUILL_API_KEY"); env != "" {
		return env, false, nil
	}
	stored, loadErr := credentials.Load()
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "warn: could not read credentials: %v\n", loadErr)
		return "", false, nil
	}
	return stored, false, nil
}
