package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fielddesc"
)

// formOnlyKeys are wire keys the CLI infers instead of taking a flag.
var formOnlyKeys = map[string]bool{"auth": true}

func lookupLocaleKey(t *testing.T, root map[string]any, dotted string) bool {
	t.Helper()
	var cur any = root
	for _, part := range strings.Split(dotted, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = m[part]; !ok {
			return false
		}
	}
	s, ok := cur.(string)
	return ok && s != ""
}

// Every descriptor needs locale keys in all locales, a docs row and a CLI flag.
func TestFieldDescriptorContract(t *testing.T) {
	cases := []struct {
		resource, docs string
		formOnly       map[string]bool
	}{
		{fielddesc.BackupTarget, "backup-target-fields.md", map[string]bool{"auth": true}},
		{fielddesc.LogDrain, "log-drain-fields.md", map[string]bool{"enabled": true}},
	}
	for _, c := range cases {
		t.Run(c.resource, func(t *testing.T) { checkContract(t, c.resource, c.docs, c.formOnly) })
	}
}

func checkContract(t *testing.T, resource, docsFile string, formOnly map[string]bool) {
	t.Helper()
	repo := filepath.Join("..", "..")
	res, ok := fielddesc.Get(resource)
	if !ok {
		t.Fatal("descriptors not registered")
	}
	docs, err := os.ReadFile(filepath.Join(repo, "docs", docsFile)) //nolint:gosec // fixed repo path
	if err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	registerDescriptorFlags(fs, resource)
	flagKeys := map[string]bool{}
	for _, f := range res.Fields {
		if f.Flag != "" {
			flagKeys[f.Key] = true
			if fs.Lookup(f.Flag) == nil {
				t.Errorf("%s: flag --%s not registered", f.ID, f.Flag)
			}
		}
	}
	langs := []string{"en", "de", "fr", "ja", "es", "pt-BR", "zh-CN"}
	locales := map[string]map[string]any{}
	load := func(lang, ns string) map[string]any {
		if m, ok := locales[lang+"/"+ns]; ok {
			return m
		}
		raw, rerr := os.ReadFile(filepath.Join(repo, "web", "src", "locales", lang, ns+".json")) //nolint:gosec // fixed repo path
		if rerr != nil {
			t.Fatal(rerr)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		locales[lang+"/"+ns] = m
		return m
	}
	for _, f := range res.Fields {
		if !flagKeys[f.Key] && !formOnly[f.Key] {
			t.Errorf("%s: key %q has no CLI flag", f.ID, f.Key)
		}
		if !strings.Contains(string(docs), "("+f.ID+")") {
			t.Errorf("%s: no docs row, run go run ./scripts/gen-field-docs", f.ID)
		}
		for _, k := range []string{f.LabelKey, f.HelpKey} {
			ns, key, found := strings.Cut(k, ":")
			if !found {
				t.Errorf("%s: key %q lacks a namespace", f.ID, k)
				continue
			}
			for _, l := range langs {
				if !lookupLocaleKey(t, load(l, ns), key) {
					t.Errorf("%s: locale %s misses %s", f.ID, l, k)
				}
			}
		}
	}
}
