package iac

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func validateYAML(t *testing.T, name string, data []byte) {
	t.Helper()
	validateYAMLWith(t, name, data, Options{})
}

func validateYAMLWith(t *testing.T, name string, data []byte, opts Options) {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: name, Data: data}})
	if len(issues) == 0 {
		_, issues = Build(docs, opts)
	}
	if len(issues) > 0 {
		t.Errorf("%s does not validate: %v\n%s", name, issues, data)
	}
}

// TestIaCExamples validates every resource block in docs/iac/*.md and every
// file under examples/iac with the real parser.
func TestIaCExamples(t *testing.T) {
	re := regexp.MustCompile("(?s)```yaml\n(version: 1\n.*?)```")
	pages, _ := filepath.Glob("../../docs/iac/*.md")
	blocks := 0
	for _, page := range pages {
		raw, err := os.ReadFile(page) //nolint:gosec // globbed docs path
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range re.FindAllStringSubmatch(string(raw), -1) {
			blocks++
			validateYAML(t, fmt.Sprintf("%s#%d", filepath.Base(page), i+1), []byte(b[1]))
		}
	}
	if blocks == 0 {
		t.Fatal("no yaml blocks found in docs/iac")
	}
	files := 0
	SetComposeSource(func() ComposeLimits {
		return ComposeLimits{Enabled: true, MaxFileBytes: 256 << 10, MaxFiles: 20, MaxTotalBytes: 2048 << 10}
	})
	t.Cleanup(func() { SetComposeSource(func() ComposeLimits { return ComposeLimits{} }) })
	attachments := exampleComposeAttachments(t)
	err := filepath.WalkDir("../../examples/iac", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		if _, isAttachment := attachments[filepath.ToSlash(path)]; isAttachment {
			return nil
		}
		raw, err := os.ReadFile(path) //nolint:gosec // walked examples path
		if err != nil {
			return err
		}
		files++
		for _, vars := range exampleVarSets(t, filepath.Dir(path)) {
			validateYAMLWith(t, path, raw, Options{Vars: vars, Attachments: exampleAttachmentsFor(attachments, path)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 2 {
		t.Fatalf("found %d example files, expected at least 2", files)
	}
}

// exampleVarSets returns one var map per values/*.yml file next to an example
// (each environment is checked on its own), or one empty set when it has none.
func exampleVarSets(t *testing.T, dir string) []map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "values", "*.yml"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(dir, ".env.example"))
	}
	if len(files) == 0 {
		return []map[string]string{nil}
	}
	var sets []map[string]string
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // globbed examples path
		if err != nil {
			t.Fatal(err)
		}
		vars := map[string]string{}
		for _, line := range strings.Split(string(raw), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
			if filepath.Base(f) == ".env.example" {
				k, v, ok = strings.Cut(strings.TrimSpace(line), "=")
			}
			if ok && k != "" && !strings.HasPrefix(k, "#") {
				vars[k] = strings.TrimSpace(v)
			}
		}
		sets = append(sets, vars)
	}
	return sets
}

func TestDocsExamplesValidate(t *testing.T) {
	var all []byte
	for _, f := range []string{"platform-as-code.md", "iac/resources.md", "stacks.md"} {
		raw, err := os.ReadFile("../../docs/" + f) //nolint:gosec // fixed docs path
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, raw...)
	}
	blocks := regexp.MustCompile("(?s)```yaml\n(version: 1\n.*?)```").FindAllStringSubmatch(string(all), -1)
	if len(blocks) < 8 {
		t.Fatalf("found %d resource examples, expected the kind references and examples", len(blocks))
	}
	for i, b := range blocks {
		docs, issues := ParseDocuments([]Source{{Name: "docs.md", Data: []byte(b[1])}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		if len(issues) > 0 {
			t.Errorf("example %d does not validate: %v\n%s", i+1, issues, b[1])
		}
	}
}

const publishedSchemaPath = "../../docs/schemas/resources.schema.json"

func TestPublishedSchemaFileIsCurrent(t *testing.T) {
	want, err := SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("IAC_UPDATE_SCHEMA") == "1" {
		if err := os.MkdirAll(filepath.Dir(publishedSchemaPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(publishedSchemaPath, want, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(publishedSchemaPath)
	if err != nil {
		t.Fatalf("read published schema: %v (run with IAC_UPDATE_SCHEMA=1 to write it)", err)
	}
	if string(got) != string(want) {
		t.Fatal("docs/schemas/resources.schema.json is stale; run: IAC_UPDATE_SCHEMA=1 go test -run TestPublishedSchemaFileIsCurrent ./internal/iac/")
	}
}

// exampleComposeAttachments finds the compose files that example documents
// reference, keyed by their walked path, so the walk skips them as documents.
func exampleComposeAttachments(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	roots, _ := filepath.Glob("../../examples/iac/*")
	for _, root := range roots {
		var srcs []Source
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) != ".yaml" {
				return err
			}
			raw, rerr := os.ReadFile(p) //nolint:gosec // walked examples path
			if rerr != nil {
				t.Fatal(rerr)
			}
			rel, _ := filepath.Rel(root, p)
			srcs = append(srcs, Source{Name: filepath.ToSlash(rel), Data: raw})
			return nil
		})
		_, atts, _ := SplitComposeRefs(srcs)
		for rel, data := range atts {
			out[filepath.ToSlash(filepath.Join(root, rel))] = data
		}
	}
	return out
}

// exampleAttachmentsFor returns the attachments of the example folder that holds path.
func exampleAttachmentsFor(all map[string][]byte, path string) map[string][]byte {
	rest := strings.TrimPrefix(filepath.ToSlash(path), "../../examples/iac/")
	root := "../../examples/iac/" + strings.SplitN(rest, "/", 2)[0] + "/"
	out := map[string][]byte{}
	for k, v := range all {
		if strings.HasPrefix(k, root) {
			out[strings.TrimPrefix(k, root)] = v
		}
	}
	return out
}
