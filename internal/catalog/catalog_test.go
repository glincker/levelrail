package catalog

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func TestTemplates_ComposeParsesAndValidates(t *testing.T) {
	seen := make(map[string]bool, len(Templates))
	for _, tpl := range Templates {
		tpl := tpl
		t.Run(tpl.ID, func(t *testing.T) {
			if tpl.ID == "" {
				t.Fatal("template has an empty ID")
			}
			if seen[tpl.ID] {
				t.Fatalf("duplicate template ID %q", tpl.ID)
			}
			seen[tpl.ID] = true

			if tpl.Name == "" {
				t.Errorf("template %q: empty Name", tpl.ID)
			}
			if tpl.Slogan == "" {
				t.Errorf("template %q: empty Slogan", tpl.ID)
			}
			if tpl.Category == "" {
				t.Errorf("template %q: empty Category", tpl.ID)
			}
			if tpl.DocumentationURL == "" {
				t.Errorf("template %q: empty DocumentationURL", tpl.ID)
			}

			if tpl.RecommendedMemoryBytes <= 0 {
				t.Errorf("template %q: RecommendedMemoryBytes must be set to a positive value", tpl.ID)
			}

			f, err := compose.Parse([]byte(tpl.Compose))
			if err != nil {
				t.Fatalf("template %q: compose.Parse() error = %v", tpl.ID, err)
			}
			if err := f.Validate(); err != nil {
				t.Fatalf("template %q: (*compose.File).Validate() error = %v", tpl.ID, err)
			}
			if _, _, err := compose.ToDesiredServices("test", f); err != nil {
				t.Fatalf("template %q: compose.ToDesiredServices() error = %v", tpl.ID, err)
			}

			if !anyServiceHasHealthcheck(f) {
				t.Errorf("template %q: no service declares a healthcheck: block", tpl.ID)
			}
		})
	}
}

func anyServiceHasHealthcheck(f *compose.File) bool {
	for _, svc := range f.Services {
		if svc.Healthcheck != nil && len(svc.Healthcheck.Test) > 0 {
			return true
		}
	}
	return false
}

func TestTemplates_MinimumCatalogSize(t *testing.T) {
	if len(Templates) < 12 {
		t.Fatalf("got %d templates, want at least 12", len(Templates))
	}
}
