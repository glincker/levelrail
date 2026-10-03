package catalog

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"gopkg.in/yaml.v3"
)

// contribFS embeds every contributor-submitted template file. See
// CONTRIBUTING.md for the format: one YAML file per template, no Go
// source or build step required to add one.
//
//go:embed contrib/*.yaml
var contribFS embed.FS

// contribTemplate is the YAML shape a contributor writes. Field names
// mirror Template's own fields, snake_case per YAML convention.
type contribTemplate struct {
	ID                     string `yaml:"id"`
	Name                   string `yaml:"name"`
	Slogan                 string `yaml:"slogan"`
	Category               string `yaml:"category"`
	DocumentationURL       string `yaml:"documentation_url"`
	Compose                string `yaml:"compose"`
	RecommendedMemoryBytes int64  `yaml:"recommended_memory_bytes"`
	RequiresGPU            bool   `yaml:"requires_gpu"`
}

// validate checks the fields a contributor must fill in. Compose parsing,
// healthcheck presence, and duplicate-ID checks across the whole catalog
// happen in catalog_test.go, the same as the Go-source entries.
func (ct contribTemplate) validate() error {
	switch {
	case ct.ID == "":
		return fmt.Errorf("missing id")
	case ct.Name == "":
		return fmt.Errorf("missing name")
	case ct.Slogan == "":
		return fmt.Errorf("missing slogan")
	case ct.Category == "":
		return fmt.Errorf("missing category")
	case ct.DocumentationURL == "":
		return fmt.Errorf("missing documentation_url")
	case ct.Compose == "":
		return fmt.Errorf("missing compose")
	case ct.RecommendedMemoryBytes <= 0:
		return fmt.Errorf("recommended_memory_bytes must be set to a positive value")
	}
	return nil
}

// contribDir is the fixed directory name every contrib file lives under,
// both in the embedded contribFS and in a test's fstest.MapFS.
const contribDir = "contrib"

// loadContribTemplates decodes every *.yaml file under contribDir in
// fsys into a Template. It only rejects a duplicate ID within contribDir
// itself; a duplicate against the Go-source catalog is caught later, by
// the whole-catalog check in catalog_test.go.
func loadContribTemplates(fsys fs.FS) ([]Template, error) {
	entries, err := fs.ReadDir(fsys, contribDir)
	if err != nil {
		return nil, fmt.Errorf("catalog: read contrib dir %q: %w", contribDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]Template, 0, len(names))
	seenID := make(map[string]string, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, contribDir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("catalog: read %s: %w", name, err)
		}

		var ct contribTemplate
		if err := yaml.Unmarshal(raw, &ct); err != nil {
			return nil, fmt.Errorf("catalog: parse %s: %w", name, err)
		}
		if err := ct.validate(); err != nil {
			return nil, fmt.Errorf("catalog: %s: %w", name, err)
		}

		if prev, dup := seenID[ct.ID]; dup {
			return nil, fmt.Errorf("catalog: %s: duplicate template id %q (already used by %s)", name, ct.ID, prev)
		}
		seenID[ct.ID] = name

		out = append(out, Template(ct))
	}
	return out, nil
}

// contribTemplates is every contrib/*.yaml template, decoded once at
// package init. A malformed file is a build-breaking error: the same
// fail-fast severity as a bad embedded migration, not a silent drop.
var contribTemplates = mustLoadContribTemplates()

func mustLoadContribTemplates() []Template {
	tpls, err := loadContribTemplates(contribFS)
	if err != nil {
		panic(err)
	}
	return tpls
}
