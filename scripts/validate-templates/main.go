// Command validate-templates checks template manifests and, optionally, a
// directory of raw compose files against the same parse, guard and
// translation path a real deploy uses. A catalog import batch runs it
// over every candidate before converting any of them into a manifest.
//
// Usage:
//
//	go run ./scripts/validate-templates [-dir internal/catalog/manifests] [-compose-dir path] [-json] [-strict]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

type result struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-templates", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "internal/catalog/manifests", "directory of <id>/template.yaml manifests")
	composeDir := fs.String("compose-dir", "", "optional directory of raw compose files (*.yaml, *.yml) to check")
	asJSON := fs.Bool("json", false, "print results as JSON")
	strict := fs.Bool("strict", false, "fail when any raw compose file fails, not only manifests")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	results := validateManifests(*dir)
	if *composeDir != "" {
		raw, err := validateComposeDir(*composeDir)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		results = append(results, raw...)
	}
	printResults(stdout, results, *asJSON)

	for _, r := range results {
		if !r.OK && (r.Kind == "manifest" || *strict) {
			return 1
		}
	}
	return 0
}

func validateManifests(dir string) []result {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []result{{Kind: "manifest", Name: dir, Reason: err.Error()}}
	}
	fsys := os.DirFS(dir)
	var out []result
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r := result{Kind: "manifest", Name: e.Name(), OK: true}
		if _, err := catalog.LoadManifest(fsys, e.Name()); err != nil {
			r.OK, r.Reason = false, flatten(err)
		}
		out = append(out, r)
	}
	return out
}

func validateComposeDir(dir string) ([]result, error) {
	var out []result
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := filepath.Ext(path)
		if d.IsDir() || (ext != ".yaml" && ext != ".yml") {
			return nil
		}
		body, err := os.ReadFile(path) //nolint:gosec // operator-supplied directory, read only
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		r := result{Kind: "compose", Name: rel, OK: true}
		if _, err := catalog.ValidateCompose(body); err != nil {
			r.OK, r.Reason = false, flatten(err)
		}
		out = append(out, r)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return out, nil
}

func flatten(err error) string {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		parts := make([]string, 0, len(joined.Unwrap()))
		for _, e := range joined.Unwrap() {
			parts = append(parts, flatten(e))
		}
		return strings.Join(parts, "; ")
	}
	return strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " ")
}

func printResults(w io.Writer, results []result, asJSON bool) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Kind != results[j].Kind {
			return results[i].Kind > results[j].Kind
		}
		return results[i].Name < results[j].Name
	})
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	var pass, fail int
	for _, r := range results {
		status := "ok"
		if !r.OK {
			status, fail = "FAIL", fail+1
		} else {
			pass++
		}
		line := fmt.Sprintf("%-4s %-8s %s", status, r.Kind, r.Name)
		if r.Reason != "" {
			line += ": " + r.Reason
		}
		_, _ = fmt.Fprintln(w, line)
	}
	_, _ = fmt.Fprintf(w, "\n%d passed, %d failed\n", pass, fail)
}
