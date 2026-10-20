package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, routes []route, groups map[string]string) map[string]any {
	t.Helper()
	raw, err := genOpenAPI31(routes, groups, brandInfo{Name: "Acme", SupportEmail: "help@example.com", DocsURL: "https://example.com"})
	if err != nil {
		t.Fatalf("genOpenAPI31: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return doc
}

func op(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()
	p, ok := doc["paths"].(map[string]any)[path].(map[string]any)
	if !ok {
		t.Fatalf("path %q missing", path)
	}
	o, ok := p[method].(map[string]any)
	if !ok {
		t.Fatalf("%s %s missing", method, path)
	}
	return o
}

func TestGenOpenAPI31(t *testing.T) {
	routes := []route{
		{method: "GET", path: "/api/v1/apps/{name}", ability: "AbilityRead", handler: "handleGetApp", description: "Fetch one app. Extra detail."},
		{method: "POST", path: "/api/v1/apps/{name}/deploy", ability: "AbilityDeploy", handler: "handleDeploy"},
		{method: "GET", path: "/api/v1/brand", ability: "Public", handler: "handleBrand"},
		{method: "GET", path: "/api/v1/me", ability: "Session", handler: "handleGetApp"},
		{method: "GET", path: "/api/v1/files/{path...}", ability: "AbilityReadSensitive", handler: "handleFile"},
		{method: "GET", path: "/api/v1/nohandler", ability: "AbilityRead"},
	}
	groups := map[string]string{"GET /api/v1/apps/{name}": "Apps"}
	doc := decode(t, routes, groups)

	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}

	get := op(t, doc, "/api/v1/apps/{name}", "get")
	if get["summary"] != "Fetch one app." {
		t.Errorf("summary = %v", get["summary"])
	}
	if get["x-required-ability"] != "read" {
		t.Errorf("ability = %v", get["x-required-ability"])
	}
	if params, _ := get["parameters"].([]any); len(params) != 1 {
		t.Errorf("want one path parameter, got %v", get["parameters"])
	}

	deploy := op(t, doc, "/api/v1/apps/{name}/deploy", "post")
	if _, ok := deploy["requestBody"]; !ok {
		t.Error("POST should declare a generic request body")
	}
	if deploy["x-required-ability"] != "deploy" {
		t.Errorf("deploy ability = %v", deploy["x-required-ability"])
	}

	pub := op(t, doc, "/api/v1/brand", "get")
	if sec, ok := pub["security"].([]any); !ok || len(sec) != 0 {
		t.Errorf("public route must override security with an empty list, got %v", pub["security"])
	}
	if !strings.Contains(pub["description"].(string), "No authentication required") {
		t.Errorf("public description = %v", pub["description"])
	}

	if op(t, doc, "/api/v1/me", "get")["operationId"] == get["operationId"] {
		t.Error("operationIds must be unique even when a handler is shared")
	}

	if got := op(t, doc, "/api/v1/nohandler", "get")["summary"]; got != "GET /api/v1/nohandler" {
		t.Errorf("empty-handler summary = %v", got)
	}

	if _, ok := op(t, doc, "/api/v1/files/{path}", "get")["parameters"]; !ok {
		t.Error("wildcard {path...} should become a {path} parameter")
	}
}

func TestGenOpenAPI31ListAndIdempotencyParams(t *testing.T) {
	routes := []route{
		{method: "GET", path: "/api/v1/apps", ability: "AbilityRead", handler: "handleListApps"},
		{method: "POST", path: "/api/v1/apps", ability: "AbilityWrite", handler: "handleCreateApp", idempotent: true},
		{method: "POST", path: "/api/v1/other", ability: "AbilityWrite", handler: "handleOther"},
	}
	doc := decode(t, routes, nil)
	refs := func(o map[string]any) string {
		var out []string
		params, _ := o["parameters"].([]any)
		for _, p := range params {
			out = append(out, p.(map[string]any)["$ref"].(string))
		}
		return strings.Join(out, ",")
	}
	if got := refs(op(t, doc, "/api/v1/apps", "get")); !strings.Contains(got, "/Limit") || !strings.Contains(got, "/Offset") {
		t.Errorf("paged route refs = %q", got)
	}
	if got := refs(op(t, doc, "/api/v1/apps", "post")); !strings.HasSuffix(got, "/IdempotencyKey") {
		t.Errorf("idempotent route refs = %q", got)
	}
	if _, ok := op(t, doc, "/api/v1/other", "post")["parameters"]; ok {
		t.Error("plain route must not gain parameters")
	}
	comps := doc["components"].(map[string]any)["parameters"].(map[string]any)
	for _, k := range []string{"Limit", "Offset", "IdempotencyKey"} {
		if _, ok := comps[k]; !ok {
			t.Errorf("missing components.parameters.%s", k)
		}
	}
}

func TestHumanizeHandler(t *testing.T) {
	tests := map[string]string{
		"handleListGPUNodes": "List GPU Nodes",
		"handleGetApp":       "Get App",
		"":                   "",
	}
	for in, want := range tests {
		if got := humanizeHandler(in); got != want {
			t.Errorf("humanizeHandler(%q) = %q, want %q", in, got, want)
		}
	}
}
