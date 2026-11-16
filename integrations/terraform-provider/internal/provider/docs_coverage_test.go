package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var docAttrLine = regexp.MustCompile("(?m)^- `([a-z0-9_.]+)` \\(")

// flattenAttrs lists attribute names as dotted paths, descending into nested attributes by reflection.
func flattenAttrs(prefix string, attrs map[string]any, out map[string]bool) {
	for name, a := range attrs {
		out[prefix+name] = true
		v := reflect.ValueOf(a)
		if f := v.FieldByName("NestedObject"); f.IsValid() {
			v = f
		} else if v.Type().Name() != "SingleNestedAttribute" {
			continue
		}
		m := v.FieldByName("Attributes")
		nested := map[string]any{}
		for _, k := range m.MapKeys() {
			nested[k.String()] = m.MapIndex(k).Interface()
		}
		flattenAttrs(prefix+name+".", nested, out)
	}
}

// checkDocPage compares the documented attribute names with the schema in both directions.
func checkDocPage(t *testing.T, file string, attrs map[string]any) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Errorf("missing docs page %s", file)
		return
	}
	text := string(b)
	if !strings.HasPrefix(text, "---\npage_title:") || !strings.Contains(text, "\nsubcategory:") || !strings.Contains(text, "\ndescription:") {
		t.Errorf("%s lacks page_title, subcategory and description front matter", file)
	}
	want := map[string]bool{}
	flattenAttrs("", attrs, want)
	got := map[string]bool{}
	for _, m := range docAttrLine.FindAllStringSubmatch(text, -1) {
		got[m[1]] = true
		if !want[m[1]] {
			t.Errorf("%s documents %q which is not in the schema", file, m[1])
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("%s does not document attribute %q", file, name)
		}
	}
}

// TestDocsCoverage fails when a registered resource, data source or provider setting has no registry page, or a page drifts from its schema.
func TestDocsCoverage(t *testing.T) {
	ctx := context.Background()
	docs := filepath.Join("..", "..", "docs")
	p := providerImpl()

	var ps provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &ps)
	pa := map[string]any{}
	for k, v := range ps.Schema.Attributes {
		pa[k] = v
	}
	checkDocPage(t, filepath.Join(docs, "index.md"), pa)

	for _, mk := range p.Resources(ctx) {
		r := mk()
		var m resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: TypeName}, &m)
		var s resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &s)
		attrs := map[string]any{}
		for k, v := range s.Schema.Attributes {
			attrs[k] = v
		}
		name := strings.TrimPrefix(m.TypeName, TypeName+"_")
		checkDocPage(t, filepath.Join(docs, "resources", name+".md"), attrs)
	}
	for _, mk := range p.DataSources(ctx) {
		d := mk()
		var m datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: TypeName}, &m)
		var s datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &s)
		attrs := map[string]any{}
		for k, v := range s.Schema.Attributes {
			attrs[k] = v
		}
		name := strings.TrimPrefix(m.TypeName, TypeName+"_")
		checkDocPage(t, filepath.Join(docs, "data-sources", name+".md"), attrs)
	}
}
