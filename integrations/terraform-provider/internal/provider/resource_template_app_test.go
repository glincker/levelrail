package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestTemplateAppSchema(t *testing.T) {
	var s resource.SchemaResponse
	NewTemplateAppResource().Schema(context.Background(), resource.SchemaRequest{}, &s)
	if d := s.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("invalid schema: %v", d)
	}
	a := s.Schema.Attributes
	if !a["template"].IsRequired() || !a["env"].IsSensitive() || !a["env"].IsOptional() || !a["domains"].IsOptional() || !a["name"].IsComputed() {
		t.Fatalf("unexpected attribute flags: %+v", a)
	}
	if a["domains"].IsSensitive() {
		t.Fatal("domains must not be sensitive")
	}
}
