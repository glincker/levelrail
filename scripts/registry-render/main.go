// Command registry-render prints a registry template's compose file after the
// same engine steps a real deploy runs, so plain `docker compose up` can start it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

const sampleValue = "sample-value"

func main() {
	id := flag.String("id", "", "template id under registry/templates")
	dir := flag.String("dir", "", "template folder (overrides -id)")
	flag.Parse()
	folder := *dir
	if folder == "" && *id != "" {
		folder = filepath.Join("registry", "templates", *id)
	}
	if folder == "" {
		fmt.Fprintln(os.Stderr, "registry-render: pass -id or -dir")
		os.Exit(2)
	}
	out, err := renderFolder(folder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "registry-render: %s: %v\n", folder, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

// renderFolder loads the folder through the catalog loader (copied into a temp
// parent, since the loader reads a parent of template folders).
func renderFolder(folder string) (string, error) {
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	parent, err := os.MkdirTemp("", "registry-render-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(parent) //nolint:errcheck // best-effort temp cleanup
	if err := copyFolder(abs, filepath.Join(parent, filepath.Base(abs))); err != nil {
		return "", err
	}
	tpls, err := catalog.LoadRegistryDir(parent)
	if err != nil {
		return "", err
	}
	if len(tpls) != 1 {
		return "", fmt.Errorf("expected one template, got %d", len(tpls))
	}
	body := tpls[0].Compose
	if m := tpls[0].Manifest; m != nil {
		// Defaults and generated prompts apply; required prompts without one get a sample.
		in := catalog.InstallInput{Env: map[string]string{}, Domains: map[string]string{}}
		for _, p := range m.Env {
			if p.Required && p.Default == "" && p.Generate == "" {
				in.Env[p.Key] = sampleValue
			}
		}
		if body, err = m.Render(in, false); err != nil {
			return "", err
		}
	}
	return render(body)
}

// render runs the engine's parse and magic-var resolution (a rejection is a
// real finding), then its text translation, and declares named volumes.
func render(body string) (string, error) {
	f, err := compose.Parse([]byte(body))
	if err != nil {
		return "", err
	}
	stub := func(string, string, int) (string, error) { return "x", nil }
	persist := func(string, string, string) error { return nil }
	_, unresolved, err := compose.ResolveMagicVars(f, stub, persist)
	if err != nil {
		return "", err
	}
	if len(unresolved) > 0 {
		return "", fmt.Errorf("unresolved variable %s", unresolved[0].String())
	}
	// Secrets are random per run at the right length; the engine owns generation.
	tr, err := compose.TranslateMagicVars(body, func(key string) string { return strings.ToLower(key) })
	if err != nil {
		return "", err
	}
	if len(tr.Unsupported) > 0 {
		return "", fmt.Errorf("unsupported variable %s", tr.Unsupported[0])
	}
	return declareVolumes(tr.Compose, f)
}

func declareVolumes(body string, f *compose.File) (string, error) {
	named := map[string]bool{}
	for _, svc := range f.Services {
		for _, v := range svc.Volumes {
			if v.Name != "" && v.HostPath == "" {
				named[v.Name] = true
			}
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", err
	}
	root := doc.Content[0]
	var vols *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "volumes" {
			vols = root.Content[i+1]
		}
	}
	if vols == nil {
		vols = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "volumes"}, vols)
	}
	have := map[string]bool{}
	for i := 0; i+1 < len(vols.Content); i += 2 {
		have[vols.Content[i].Value] = true
	}
	names := make([]string, 0, len(named))
	for n := range named {
		if !have[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		vols.Content = append(vols.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: n},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle})
	}
	out, err := yaml.Marshal(&doc)
	return string(out), err
}

func copyFolder(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name())) //nolint:gosec // folder chosen by the operator
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o600); err != nil { //nolint:gosec // names come from ReadDir of the chosen folder
			return err
		}
	}
	return nil
}
