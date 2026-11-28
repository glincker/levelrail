package platformimport

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/iac"
)

func TestEmitDoesNotInventAPort(t *testing.T) {
	files, err := EmitStack(EmitInput{Source: "test", Apps: []EmitApp{{Name: "cache", Image: "redis:7", PortlessOK: true}}}, EmitOptions{Stack: "s", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(string(f.Content), "port: 80") {
			t.Fatalf("invented a port in %s:\n%s", f.Name, f.Content)
		}
	}
}

func TestEmitPortlessAppIsGuidanceAndKeepsOthers(t *testing.T) {
	in := EmitInput{Source: "test", Apps: []EmitApp{{Name: "cache", Image: "redis:7", PortlessOK: true}, {Name: "web", Image: "nginx", Port: 8080}}}
	files, err := EmitStack(in, EmitOptions{Stack: "s", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	var srcs []iac.Source
	for _, f := range files {
		if f.Name == EmitAppsFile {
			if !strings.Contains(string(f.Content), "# no port in the source") || !strings.Contains(string(f.Content), "name: web") {
				t.Fatalf("want guidance for cache and web kept:\n%s", f.Content)
			}
		}
		if strings.HasSuffix(f.Name, ".yaml") && f.Name != EmitStackFile {
			srcs = append(srcs, iac.Source{Name: f.Name, Data: f.Content})
		}
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		_, issues = iac.Build(docs, iac.Options{})
	}
	if len(issues) > 0 {
		t.Fatalf("output must validate: %v", issues)
	}
}

func TestEmitAssumesPortWhenPortlessNotAllowed(t *testing.T) {
	files, err := EmitStack(EmitInput{Source: "test", Apps: []EmitApp{{Name: "web", Image: "nginx"}}}, EmitOptions{Stack: "s", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		found = found || strings.Contains(string(f.Content), "port: 80")
	}
	if !found {
		t.Fatal("expected the default port for sources that always have one")
	}
}
