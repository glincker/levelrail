package catalog

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/GLINCKER/levelrail/internal/compose"
	"gopkg.in/yaml.v3"
)

//go:embed manifests
var manifestFS embed.FS

const (
	manifestDir         = "manifests"
	manifestFile        = "template.yaml"
	defaultComposeFile  = "compose.yaml"
	sampleRequiredValue = "example"
	sampleDomain        = "app.example.com"
)

// LoadManifests reads every <dir>/<id>/template.yaml in fsys, sorted by id,
// and validates each one. All problems are reported, not just the first.
func LoadManifests(fsys fs.FS, dir string) ([]Manifest, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("catalog: read manifests dir %q: %w", dir, err)
	}
	var (
		out  []Manifest
		errs []error
	)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := LoadManifest(fsys, path.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, errors.Join(errs...)
}

// LoadManifest reads and validates one template directory.
func LoadManifest(fsys fs.FS, dir string) (Manifest, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, manifestFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("catalog: %s: %w", dir, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("catalog: %s/%s: %w", dir, manifestFile, err)
	}
	if m.ComposeFile == "" {
		m.ComposeFile = defaultComposeFile
	}
	body, err := fs.ReadFile(fsys, path.Join(dir, path.Clean("/" + m.ComposeFile)[1:]))
	if err != nil {
		return Manifest{}, fmt.Errorf("catalog: %s: compose file: %w", dir, err)
	}
	m.Compose = string(body)
	if m.ID != path.Base(dir) {
		return Manifest{}, fmt.Errorf("catalog: %s: id %q must match its directory name", dir, m.ID)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("catalog: %s: %w", dir, err)
	}
	return m, nil
}

// Validate checks the manifest's fields, that its prompts and placeholders
// match one to one, and that a sample render deploys through the compose path.
func (m Manifest) Validate() error {
	var errs []error
	for field, v := range map[string]string{"name": m.Name, "description": m.Description, "category": m.Category, "documentation_url": m.DocumentationURL} {
		if v == "" {
			errs = append(errs, fmt.Errorf("missing %s", field))
		}
	}
	if !idPattern.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("id %q must be lowercase letters, digits and dashes", m.ID))
	}
	if m.RecommendedMemoryBytes <= 0 {
		errs = append(errs, fmt.Errorf("recommended_memory_bytes must be positive"))
	}
	errs = append(errs, m.validatePrompts()...)
	if len(errs) == 0 {
		errs = append(errs, m.validateRender()...)
	}
	return errors.Join(errs...)
}

func (m Manifest) validatePrompts() []error {
	var errs []error
	declared := map[string]bool{}
	for _, p := range m.Env {
		switch {
		case !envKeyPattern.MatchString(p.Key):
			errs = append(errs, fmt.Errorf("env key %q must be UPPER_SNAKE_CASE", p.Key))
		case declared[p.Key]:
			errs = append(errs, fmt.Errorf("env key %q declared twice", p.Key))
		case p.Generate != "" && generateKinds[p.Generate] == "":
			errs = append(errs, fmt.Errorf("env %s: generate %q must be one of password, user, hex, base64", p.Key, p.Generate))
		case p.Generate != "" && p.Default != "":
			errs = append(errs, fmt.Errorf("env %s: set generate or default, not both", p.Key))
		}
		declared[p.Key] = true
	}
	used := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(m.Compose, -1) {
		key := match[1]
		if len(key) > 8 && key[:8] == "SERVICE_" {
			continue
		}
		used[key] = true
		if !declared[key] {
			errs = append(errs, fmt.Errorf("compose uses ${%s} but env declares no prompt for it", key))
		}
	}
	for key := range declared {
		if !used[key] {
			errs = append(errs, fmt.Errorf("env prompt %s is never used in the compose file", key))
		}
	}
	return errs
}

func (m Manifest) validateRender() []error {
	in := InstallInput{Env: map[string]string{}, Domains: map[string]string{}}
	for _, p := range m.Env {
		if p.Required && p.Default == "" && p.Generate == "" {
			in.Env[p.Key] = sampleRequiredValue
		}
	}
	for _, d := range m.Domains {
		in.Domains[d.Service] = sampleDomain
	}
	body, err := m.Render(in, false)
	if err != nil {
		return []error{err}
	}
	services, err := ValidateCompose([]byte(body))
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, d := range m.Domains {
		if !services[d.Service] {
			errs = append(errs, fmt.Errorf("domain prompt names service %q, which the compose file does not declare", d.Service))
		}
	}
	if m.Healthcheck != nil && !services[m.Healthcheck.Service] {
		errs = append(errs, fmt.Errorf("healthcheck names service %q, which the compose file does not declare", m.Healthcheck.Service))
	}
	return errs
}

// ValidateCompose runs a compose body through the same parse, magic-var
// and translation steps a real deploy uses, with stand-in secrets, and
// returns the declared service keys. A later catalog import batch runs
// this over every candidate file (scripts/validate-templates).
func ValidateCompose(body []byte) (map[string]bool, error) {
	f, err := compose.Parse(body)
	if err != nil {
		return nil, err
	}
	if _, err := f.ApplyProfiles(nil); err != nil {
		return nil, err
	}
	stub := func(string, string, int) (string, error) { return "x", nil }
	persist := func(string, string, string) error { return nil }
	_, unresolved, err := compose.ResolveMagicVars(f, stub, persist)
	if err != nil {
		return nil, err
	}
	if len(unresolved) > 0 {
		return nil, fmt.Errorf("unresolved variable %s", unresolved[0].String())
	}
	desired, _, err := compose.ToDesiredServices("validate", f)
	if err != nil {
		return nil, err
	}
	if err := checkTemplateFiles(desired); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(f.Services))
	for k := range f.Services {
		out[k] = true
	}
	return out, nil
}

// manifestTemplates is every embedded manifest, loaded once at init; a bad
// manifest is a build-breaking error, the same severity as contrib files.
var manifestTemplates = mustLoadManifestTemplates()

func mustLoadManifestTemplates() []Template {
	ms, err := LoadManifests(manifestFS, manifestDir)
	if err != nil {
		panic(err)
	}
	out := make([]Template, 0, len(ms))
	for _, m := range ms {
		t, err := m.Template()
		if err != nil {
			panic(err)
		}
		out = append(out, t)
	}
	return out
}
