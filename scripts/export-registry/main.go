// Command export-registry writes every catalog.Templates entry to
// registry/templates/<id>/ (template.yaml plus compose.yaml).
//
// Usage (from the repo root): go run ./scripts/export-registry
// Pass -check to exit non-zero instead of writing when the folder is stale.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// render maps relative file path to content for the whole registry.
func render(templates []catalog.Template) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, t := range templates {
		meta, compose, mc, err := catalog.EncodeRegistryTemplate(t)
		if err != nil {
			return nil, err
		}
		files[filepath.Join(t.ID, catalog.RegistryMetaFile)] = meta
		files[filepath.Join(t.ID, catalog.RegistryComposeFile)] = compose
		if mc != nil {
			files[filepath.Join(t.ID, catalog.RegistryManifestComposeFile)] = mc
		}
	}
	return files, nil
}

func existing(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p) //nolint:gosec // repo-local registry path
		files[rel] = b
		return err
	})
	return files, err
}

func main() {
	out := flag.String("out", "registry/templates", "registry directory to write")
	check := flag.Bool("check", false, "fail instead of writing when the folder is stale")
	flag.Parse()

	want, err := render(catalog.Templates)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	have, err := existing(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var stale []string
	for p, b := range want {
		if !bytes.Equal(have[p], b) {
			stale = append(stale, p)
		}
	}
	for p := range have {
		if _, ok := want[p]; !ok {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	if *check {
		if len(stale) > 0 {
			fmt.Fprintf(os.Stderr, "%s is stale (%d files, first %s): run go run ./scripts/export-registry\n", *out, len(stale), stale[0])
			os.Exit(1)
		}
		return
	}
	for _, p := range stale {
		full := filepath.Join(*out, p)
		if b, ok := want[p]; ok {
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { //nolint:gosec // repo data
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if err := os.WriteFile(full, b, 0o644); err != nil { //nolint:gosec // repo data
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else if err := os.Remove(full); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
