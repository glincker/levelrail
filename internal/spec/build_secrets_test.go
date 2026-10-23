package spec

import (
	"strings"
	"testing"
)

func TestParse_BuildSecrets(t *testing.T) {
	t.Setenv(EnvBuildSecretsMax, "2")
	doc := func(list string) []byte {
		return []byte("version: 1\nservices:\n  web: { build: { type: " + list + " }, port: 8080 }\n")
	}
	s, err := Parse(doc("dockerfile, secrets: [NPM_TOKEN, GH_TOKEN]"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := s.Services["web"].Build.Secrets; len(got) != 2 || got[0] != "NPM_TOKEN" {
		t.Errorf("Secrets = %v", got)
	}
	cases := map[string]string{
		"dockerfile, secrets: [A, A]":     "are equal",
		"dockerfile, secrets: [A, B, C]":  "over the cap of 2",
		"dockerfile, secrets: [\"1BAD\"]": "does not match pattern",
		"railpack, secrets: [A]":          "build.secrets is not meaningful",
		"static, secrets: [A]":            "build.secrets is not meaningful",
	}
	// The schema rejects bad names and duplicates before the Go checks run.
	for in, want := range cases {
		if _, err := Parse(doc(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", in, err, want)
		}
	}
}

func TestValidateBuildSecrets_GoChecks(t *testing.T) {
	b := Build{Type: BuildDockerfile, Secrets: []string{"A", "A"}}
	if err := b.validateBuildSecrets("web"); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("dup err = %v", err)
	}
	b.Secrets = []string{"1BAD"}
	if err := b.validateBuildSecrets("web"); err == nil || !strings.Contains(err.Error(), "not a valid variable name") {
		t.Errorf("name err = %v", err)
	}
}
