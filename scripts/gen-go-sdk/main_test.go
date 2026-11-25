package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Fails when docs/public/openapi.json changed without regenerating the Go SDK.
func TestGeneratedSDKMatchesSpec(t *testing.T) {
	root := filepath.Join("..", "..")
	in, err := os.ReadFile(filepath.Join(root, specRel)) //nolint:gosec // fixed repo-relative path
	if err != nil {
		t.Fatal(err)
	}
	want, err := render(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, outRel)) //nolint:gosec // fixed repo-relative path
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale, run: go run ./scripts/gen-go-sdk", outRel)
	}
}
