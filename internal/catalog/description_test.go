package catalog

import (
	"regexp"
	"testing"
)

// Operator-supplied env (empty default, required marker or no default) needs
// setup notes, otherwise the user never learns what to set.
var operatorEnvRef = regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*(:-\}|:\?[^}]*\}|\})`)

func TestTemplatesWithOperatorEnvHaveDescription(t *testing.T) {
	for _, tpl := range Templates {
		if tpl.Manifest != nil {
			continue
		}
		if operatorEnvRef.MatchString(tpl.Compose) && tpl.Description == "" {
			t.Errorf("template %q needs operator env but has no Description", tpl.ID)
		}
	}
}

func TestManifestTemplateDescriptionKeepsSlogan(t *testing.T) {
	for _, tpl := range Templates {
		if tpl.Manifest != nil && (tpl.Description != tpl.Manifest.Description || tpl.Slogan != tpl.Manifest.Description) {
			t.Fatalf("template %q: description/slogan diverge from manifest", tpl.ID)
		}
	}
}
