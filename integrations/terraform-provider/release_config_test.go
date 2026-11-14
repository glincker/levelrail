package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRegistryManifestParses(t *testing.T) {
	raw, err := os.ReadFile("terraform-registry-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Version  int `json:"version"`
		Metadata struct {
			Protocols []string `json:"protocol_versions"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != 1 || len(m.Metadata.Protocols) != 1 || m.Metadata.Protocols[0] != "6.0" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

// The module has no YAML dependency, so the goreleaser file is checked as text.
func TestGoreleaserConfig(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.ContainsRune(text, '\t') {
		t.Fatal("tabs are not valid YAML indentation")
	}
	for _, want := range []string{
		"CGO_ENABLED=0", "-trimpath", "-X main.version={{ .Version }}",
		"{{ .ProjectName }}_{{ .Version }}_SHA256SUMS", "{{ .Env.GPG_FINGERPRINT }}",
		"draft: true", "disable: true", "project_name: terraform-provider-",
		"terraform-registry-manifest.json",
	} {
		if !strings.Contains(text, want) {
			t.Errorf(".goreleaser.yml is missing %q", want)
		}
	}
	if strings.Contains(text, "PRIVATE KEY") {
		t.Fatal("key material must never be in the config")
	}
	for _, p := range []string{"main.go", "terraform-registry-manifest.json", "release/mirror-sync.sh", "release/workflow.yml.example"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("referenced path missing: %s", p)
		}
	}
}
