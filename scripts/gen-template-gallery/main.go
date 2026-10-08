// Command gen-template-gallery exports internal/catalog.Templates as the JSON
// the docs site's self-host gallery pages are built from.
//
// Usage (from the repo root): go run ./scripts/gen-template-gallery
// Pass -check to exit non-zero instead of writing when the file is stale.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// Service is one compose service as shown on a gallery page.
type Service struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Ports   []int    `json:"ports"`
	Volumes []string `json:"volumes"`
}

// EnvVar is one environment variable name and how it gets its value.
// Values are never exported.
type EnvVar struct {
	Service string `json:"service"`
	Key     string `json:"key"`
	Kind    string `json:"kind"`
}

// Entry is one template in the exported gallery data.
type Entry struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Slogan           string    `json:"slogan"`
	Category         string    `json:"category"`
	DocumentationURL string    `json:"docUrl,omitempty"`
	MemoryMiB        int64     `json:"memoryMiB,omitempty"`
	RequiresGPU      bool      `json:"gpu,omitempty"`
	NeedsConfig      bool      `json:"needsConfig"`
	Services         []Service `json:"services"`
	Env              []EnvVar  `json:"env"`
}

const (
	kindGenerated = "generated"
	kindRequired  = "required"
	kindPreset    = "preset"
)

var dashReplacer = strings.NewReplacer(" \u2014 ", ", ", "\u2014", ", ", " \u2013 ", ", ", "\u2013", "-")

// sanitize removes em and en dashes from catalog prose.
func sanitize(s string) string {
	return dashReplacer.Replace(s)
}

func detectGen(_, _ string, _ int) (string, error) { return "x", nil }
func detectPersist(_, _, _ string) error           { return nil }

// buildEntry converts one catalog template, classifying env vars by name only.
func buildEntry(t catalog.Template) (Entry, error) {
	e := Entry{
		ID:               t.ID,
		Name:             sanitize(t.Name),
		Slogan:           sanitize(t.Slogan),
		Category:         t.Category,
		DocumentationURL: t.DocumentationURL,
		MemoryMiB:        t.RecommendedMemoryBytes / (1 << 20),
		RequiresGPU:      t.RequiresGPU,
		Services:         []Service{},
		Env:              []EnvVar{},
	}
	f, err := compose.Parse([]byte(t.Compose))
	if err != nil {
		return e, fmt.Errorf("parse %s: %w", t.ID, err)
	}
	for name, svc := range f.Services {
		s := Service{Name: name, Image: svc.Image, Ports: []int{}, Volumes: []string{}}
		for _, p := range svc.Ports {
			s.Ports = append(s.Ports, p.ContainerPort)
		}
		for _, v := range svc.Volumes {
			src := v.Name
			if src == "" {
				src = "host path"
			}
			s.Volumes = append(s.Volumes, src+" -> "+v.ContainerPath)
		}
		e.Services = append(e.Services, s)
	}
	sort.Slice(e.Services, func(i, j int) bool { return e.Services[i].Name < e.Services[j].Name })

	f2, err := compose.Parse([]byte(t.Compose))
	if err != nil {
		return e, fmt.Errorf("parse %s: %w", t.ID, err)
	}
	secretEnv, unresolved, err := compose.ResolveMagicVars(f2, detectGen, detectPersist)
	if err != nil {
		return e, fmt.Errorf("resolve %s: %w", t.ID, err)
	}
	kinds := map[string]string{}
	for svc, keys := range secretEnv {
		for _, k := range keys {
			kinds[svc+"\x00"+k] = kindGenerated
		}
	}
	for _, u := range unresolved {
		kinds[u.Service+"\x00"+u.EnvKey] = kindRequired
		e.NeedsConfig = true
	}
	for name, svc := range f.Services {
		for key := range svc.Environment {
			kind := kinds[name+"\x00"+key]
			if kind == "" {
				kind = kindPreset
			}
			e.Env = append(e.Env, EnvVar{Service: name, Key: key, Kind: kind})
		}
	}
	sort.Slice(e.Env, func(i, j int) bool {
		if e.Env[i].Service != e.Env[j].Service {
			return e.Env[i].Service < e.Env[j].Service
		}
		return e.Env[i].Key < e.Env[j].Key
	})
	return e, nil
}

func generate(templates []catalog.Template) ([]byte, error) {
	out := make([]Entry, 0, len(templates))
	for _, t := range templates {
		e, err := buildEntry(t)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("encode gallery data: %w", err)
	}
	return buf.Bytes(), nil
}

func main() {
	path := flag.String("out", "docs/.vitepress/data/templates.json", "JSON file to write")
	check := flag.Bool("check", false, "fail instead of writing when the file is stale")
	flag.Parse()

	data, err := generate(catalog.Templates)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		cur, _ := os.ReadFile(*path)
		if !bytes.Equal(cur, data) {
			fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-template-gallery\n", *path)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*path, data, 0o644); err != nil { //nolint:gosec // docs data, world-readable by design
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
