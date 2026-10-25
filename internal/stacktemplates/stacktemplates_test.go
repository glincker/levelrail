package stacktemplates

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestTemplateBundlesLoadThroughIaC(t *testing.T) {
	for _, tpl := range List() {
		t.Run(tpl.Name, func(t *testing.T) {
			set := map[string]string{"name": "sample", "domain": "sample.example.com"}
			files, err := tpl.Render(set)
			if err != nil {
				t.Fatal(err)
			}
			again, _ := tpl.Render(set)
			if !reflect.DeepEqual(files, again) {
				t.Fatal("render is not deterministic")
			}
			names := make([]string, 0, len(files))
			for n := range files {
				names = append(names, n)
			}
			sort.Strings(names)
			var srcs []iac.Source
			for _, n := range names {
				if !strings.HasSuffix(n, ".yaml") {
					continue
				}
				if bytes.Contains(files[n], []byte("{{")) {
					t.Fatalf("%s keeps a placeholder", n)
				}
				if n == "stack.yaml" {
					if err := iac.ValidateStackName("sample"); err != nil || !bytes.Contains(files[n], []byte("kind: Stack")) {
						t.Fatalf("bad stack.yaml: %v", err)
					}
					continue
				}
				srcs = append(srcs, iac.Source{Name: n, Data: files[n]})
			}
			docs, issues := iac.ParseDocuments(srcs)
			if len(issues) > 0 {
				t.Fatalf("parse issues: %v", issues)
			}
			res, issues := iac.Build(docs, iac.Options{Project: "sample"})
			if len(issues) > 0 {
				t.Fatalf("build issues: %v", issues)
			}
			if len(res) == 0 {
				t.Fatal("no resources built")
			}
			for _, n := range []string{"README.md", "SECRETS.md", "stack.yaml"} {
				if _, ok := files[n]; !ok {
					t.Fatalf("missing %s", n)
				}
			}
		})
	}
}

func TestTemplateVarsRejectUnknown(t *testing.T) {
	tpl, _ := Find("agent-app")
	if _, err := tpl.Render(map[string]string{"nope": "x"}); err == nil {
		t.Fatal("want error for unknown variable")
	}
}
