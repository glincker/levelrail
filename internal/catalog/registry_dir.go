package catalog

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"sort"

	"gopkg.in/yaml.v3"
)

// Registry file names inside one registry/templates/<id>/ folder.
const (
	RegistryMetaFile    = "template.yaml"
	RegistryComposeFile = "compose.yaml"
	// RegistryManifestComposeFile holds the manifest's raw compose (with its
	// ${KEY} prompts) when that differs from the rendered default in compose.yaml.
	RegistryManifestComposeFile = "manifest.compose.yaml"
)

// RegistryMeta is the on-disk template.yaml. The compose body lives beside
// it in compose.yaml so it stays byte-for-byte.
type RegistryMeta struct {
	ID                     string   `yaml:"id"`
	Name                   string   `yaml:"name"`
	Slogan                 string   `yaml:"slogan"`
	Description            string   `yaml:"description,omitempty"`
	Category               string   `yaml:"category"`
	DocumentationURL       string   `yaml:"documentation_url"`
	RecommendedMemoryBytes int64    `yaml:"recommended_memory_bytes"`
	RequiresGPU            bool     `yaml:"requires_gpu"`
	Platforms              []string `yaml:"platforms,omitempty"`
	Icon                   string   `yaml:"icon,omitempty"`
	// Marketplace fields for registry-only templates, which have no Go literal
	// or template_meta.json row to carry them.
	SourceRepo   string    `yaml:"source_repo,omitempty"`
	License      string    `yaml:"license,omitempty"`
	LicenseClass string    `yaml:"license_class,omitempty"`
	LicenseURL   string    `yaml:"license_url,omitempty"`
	Status       string    `yaml:"status,omitempty"`
	Manifest     *Manifest `yaml:"manifest,omitempty"`
}

// EncodeRegistryTemplate returns the template.yaml and compose.yaml bytes for t,
// plus the manifest's own compose when it differs (nil otherwise).
func EncodeRegistryTemplate(t Template) (meta, compose, manifestCompose []byte, err error) {
	m := RegistryMeta{
		ID: t.ID, Name: t.Name, Slogan: t.Slogan, Description: t.Description,
		Category: t.Category, DocumentationURL: t.DocumentationURL,
		RecommendedMemoryBytes: t.RecommendedMemoryBytes, RequiresGPU: t.RequiresGPU,
		Platforms: t.Platforms, Manifest: t.Manifest,
	}
	meta, err = yaml.Marshal(m)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode template %s: %w", t.ID, err)
	}
	if t.Manifest != nil && t.Manifest.Compose != t.Compose {
		manifestCompose = []byte(t.Manifest.Compose)
	}
	return meta, []byte(t.Compose), manifestCompose, nil
}

// LoadRegistryDir reads every <dir>/<id>/ folder into templates sorted by
// ID. Registry-only ids reach Templates through registry_embed.go.
func LoadRegistryDir(dir string) ([]Template, error) {
	es, err := LoadRegistryEntries(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(es))
	for _, e := range es {
		out = append(out, e.Template)
	}
	return out, nil
}

// RegistryEntry pairs a loaded template with its on-disk metadata, so callers
// can read the marketplace fields that Template does not carry.
type RegistryEntry struct {
	Template Template
	Meta     RegistryMeta
}

// LoadRegistryEntries is LoadRegistryDir plus each folder's template.yaml meta.
func LoadRegistryEntries(dir string) ([]RegistryEntry, error) {
	return LoadRegistryEntriesFS(os.DirFS(dir)) //nolint:gosec // registry folder chosen by the caller
}

// LoadRegistryEntriesFS reads every top-level folder of fsys, so the embedded
// registry and an on-disk one load through the same code.
func LoadRegistryEntriesFS(fsys fs.FS) ([]RegistryEntry, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var out []RegistryEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		t, m, err := loadRegistryTemplate(sub, e.Name())
		if err != nil {
			return nil, err
		}
		if t.ID != e.Name() {
			return nil, fmt.Errorf("registry folder %s holds template id %q", e.Name(), t.ID)
		}
		out = append(out, RegistryEntry{Template: t, Meta: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Template.ID < out[j].Template.ID })
	return out, nil
}

func loadRegistryTemplate(folder fs.FS, name string) (Template, RegistryMeta, error) {
	metaBytes, err := fs.ReadFile(folder, RegistryMetaFile)
	if err != nil {
		return Template{}, RegistryMeta{}, fmt.Errorf("read %s: %w", name, err)
	}
	compose, err := fs.ReadFile(folder, RegistryComposeFile)
	if err != nil {
		return Template{}, RegistryMeta{}, fmt.Errorf("read %s: %w", name, err)
	}
	var m RegistryMeta
	dec := yaml.NewDecoder(bytes.NewReader(metaBytes))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Template{}, RegistryMeta{}, fmt.Errorf("parse %s: %w", name, err)
	}
	if m.Manifest != nil {
		m.Manifest.Compose = string(compose)
		if raw, err := fs.ReadFile(folder, RegistryManifestComposeFile); err == nil {
			m.Manifest.Compose = string(raw)
		}
		normalizeEmpty(reflect.ValueOf(m.Manifest).Elem())
	}
	return Template{
		ID: m.ID, Name: m.Name, Slogan: m.Slogan, Description: m.Description,
		Category: m.Category, DocumentationURL: m.DocumentationURL, Compose: string(compose),
		RecommendedMemoryBytes: m.RecommendedMemoryBytes, RequiresGPU: m.RequiresGPU,
		Platforms: m.Platforms, Manifest: m.Manifest,
	}, m, nil
}

// normalizeEmpty turns empty slices into nil, since yaml writes a nil slice
// as [] and the catalog never distinguishes the two.
func normalizeEmpty(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				normalizeEmpty(v.Field(i))
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			normalizeEmpty(v.Elem())
		}
	case reflect.Slice:
		if v.Len() == 0 {
			v.Set(reflect.Zero(v.Type()))
			return
		}
		for i := 0; i < v.Len(); i++ {
			normalizeEmpty(v.Index(i))
		}
	}
}
