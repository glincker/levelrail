package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStackRefusesTemplateAppEnvVarsAndKeepsNoValue(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	docs := "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec:\n  template: ghost\n  env: {ADMIN_PASSWORD: \"${{ env.PW }}\"}\n"
	code, out := stackApply(t, rt, cookie, docs, map[string]any{"vars": map[string]string{"PW": "s3cret-value-123"}})
	if code == http.StatusOK || out.Revision != nil {
		t.Fatalf("stack accepted TemplateApp env vars: %d %+v", code, out)
	}
	var raw map[string]any
	if c := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &raw); c == http.StatusOK && strings.Contains(strings.ToLower(toJSON(t, raw)), "s3cret-value-123") {
		t.Fatalf("revisions hold the value: %v", raw)
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
