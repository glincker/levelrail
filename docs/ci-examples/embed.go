// Package ciexamples embeds the CI pipeline examples under docs/ci-examples, so the dashboard, CLI and API serve
// exactly the files the documentation links to.
package ciexamples

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

//go:embed manifest.json *.yml token/*
var files embed.FS

// Auth modes: oidc swaps the job identity for a short-lived token, token uses a stored deployer token.
const (
	AuthOIDC  = "oidc"
	AuthToken = "token"
)

// AppPlaceholder is the app name used in every example; Content swaps it for the real app. Repository owners
// stay as OWNER/REPO so a pasted file can never name a real account.
const AppPlaceholder = "my-app"

// Template is one provider's example pipeline.
type Template struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Target string `json:"target"`
	Auth   string `json:"auth"`
}

// Templates lists the examples in manifest order.
func Templates() ([]Template, error) {
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("ciexamples: read manifest: %w", err)
	}
	var out []Template
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ciexamples: parse manifest: %w", err)
	}
	return out, nil
}

// Content returns the example text with the placeholder app name replaced by app.
func Content(t Template, app string) (string, error) {
	if err := spec.ValidateServiceName(app); err != nil {
		return "", fmt.Errorf("ciexamples: %w", err)
	}
	raw, err := files.ReadFile(t.File)
	if err != nil {
		return "", fmt.Errorf("ciexamples: read %s: %w", t.File, err)
	}
	return strings.ReplaceAll(string(raw), AppPlaceholder, app), nil
}
