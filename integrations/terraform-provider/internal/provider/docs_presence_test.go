package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// repoFile reads a file relative to the repository root, which sits three levels above this package.
func repoFile(t *testing.T, rel ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "..", ".."}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func repoExamples(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "examples", "terraform-provider")
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples in %s: %v", dir, err)
	}
	var all strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

// TestDocsAndExamplesCoverEverything fails with the names that have no docs row or no example block.
func TestDocsAndExamplesCoverEverything(t *testing.T) {
	ctx := context.Background()
	docs, examples := repoFile(t, "docs", "terraform-provider.md"), repoExamples(t)
	p := providerImpl()
	check := func(kind, name string) {
		if !strings.Contains(docs, "`"+name+"`") {
			t.Errorf("%s %s has no entry in docs/terraform-provider.md", kind, name)
		}
		if !strings.Contains(examples, kind+" \""+name+"\"") {
			t.Errorf("%s %s has no example block in examples/terraform-provider", kind, name)
		}
	}
	for _, mk := range p.Resources(ctx) {
		var m resource.MetadataResponse
		mk().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: TypeName}, &m)
		check("resource", m.TypeName)
	}
	for _, mk := range p.DataSources(ctx) {
		var m datasource.MetadataResponse
		mk().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: TypeName}, &m)
		check("data", m.TypeName)
	}
}

// TestDocsMentionEveryProviderSetting keeps provider block docs from drifting behind the schema.
func TestDocsMentionEveryProviderSetting(t *testing.T) {
	docs := repoFile(t, "docs", "terraform-provider.md")
	for _, name := range []string{"max_retries", "retry_wait_max_seconds"} {
		if !strings.Contains(docs, "`"+name+"`") {
			t.Errorf("provider setting %s is not documented", name)
		}
	}
}
