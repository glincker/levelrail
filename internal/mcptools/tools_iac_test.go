package mcptools

import (
	"reflect"
	"strings"
	"testing"
)

func TestIaCToolsCarryNoPlaceholderValues(t *testing.T) {
	typ := reflect.TypeOf(planApplyInput{})
	for i := 0; i < typ.NumField(); i++ {
		if tag := typ.Field(i).Tag.Get("json"); strings.HasPrefix(tag, "vars") || strings.HasPrefix(tag, "secrets") {
			t.Fatalf("planApplyInput exposes %q; placeholder and secret values must not come from a tool call", tag)
		}
	}
	req := planApplyInput{Files: []iacFileInput{{Name: "a.yaml", Content: "x: ${{ env.HOME }}"}}}.request()
	if req.Vars != nil || req.Secrets != nil {
		t.Fatalf("request carries vars %v secrets %v", req.Vars, req.Secrets)
	}
}
