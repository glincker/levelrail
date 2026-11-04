package iac

import (
	"strings"
	"testing"
)

const placeholderApp = "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: \"ghcr.io/acme/web:${{ env.TAG }}\"}\n    port: 80\n"

func TestImagePlaceholderIsSubstituted(t *testing.T) {
	res := mustBuild(t, placeholderApp, Options{Vars: map[string]string{"TAG": "1.4.0"}})
	if got := stringField(res[0].Fields, "image"); got != "ghcr.io/acme/web:1.4.0" {
		t.Fatalf("image = %q", got)
	}
}

func TestUnresolvedPlaceholdersAreRejected(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"missing image var": {placeholderApp, "TAG is not provided"},
		"outside allowlist": {"version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  project: \"${{ env.P }}\"\n  service:\n    build: {type: image, image: x}\n    port: 80\n", "unresolved placeholder"},
		"secret in label":   {"version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    labels: {a: \"${{ secrets.X }}\"}\n", "unresolved placeholder"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			docs, pi := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(tc.src)}})
			if len(pi) > 0 {
				t.Fatal(pi)
			}
			res, issues := Build(docs, Options{})
			if len(issues) == 0 || res != nil {
				t.Fatalf("want issues, got res=%v issues=%v", res, issues)
			}
			if !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("message = %q, want %q", issues[0].Message, tc.want)
			}
		})
	}
}

const allowlistApp = "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    replicas: \"${{ env.REPLICAS }}\"\n    resources: {cpu: \"${{ env.CPU }}\", memory: \"${{ env.MEM }}\"}\n    domains: [\"${{ env.HOST }}.example.com\"]\n    labels: {tier: \"${{ env.TIER }}\"}\n"

func TestPlaceholderAllowlist(t *testing.T) {
	vars := map[string]string{"REPLICAS": "3", "CPU": "0.5", "MEM": "512Mi", "HOST": "app", "TIER": "gold"}
	docs, pi := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(allowlistApp)}})
	if len(pi) > 0 {
		t.Fatal(pi)
	}
	res, issues := Build(docs, Options{Vars: vars})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	if got := res[0].Fields["replicas"]; got != float64(3) && got != 3 {
		t.Fatalf("replicas = %#v", got)
	}
	if d := stringList(res[0].Fields["domains"]); len(d) != 1 || d[0] != "app.example.com" {
		t.Fatalf("domains = %v", res[0].Fields["domains"])
	}
	// The same documents built again with other values must not see the first render.
	vars["REPLICAS"] = "5"
	res, issues = Build(docs, Options{Vars: vars})
	if len(issues) > 0 || (res[0].Fields["replicas"] != float64(5) && res[0].Fields["replicas"] != 5) {
		t.Fatalf("second build: %v %v", res, issues)
	}
}

func TestPlaceholderAllowlistWrongTypeNamesFieldNotValue(t *testing.T) {
	docs, pi := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(allowlistApp)}})
	if len(pi) > 0 {
		t.Fatal(pi)
	}
	_, issues := Build(docs, Options{Vars: map[string]string{"REPLICAS": "many-secretish", "CPU": "1", "MEM": "1g", "HOST": "a", "TIER": "b"}})
	if len(issues) == 0 || issues[0].Path != "spec.service.replicas" {
		t.Fatalf("issues = %v", issues)
	}
	for _, is := range issues {
		if strings.Contains(is.Message, "many-secretish") {
			t.Fatalf("message leaks the value: %q", is.Message)
		}
	}
	_, issues = Build(docs, Options{Vars: map[string]string{}})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "is not provided") {
		t.Fatalf("missing var issues = %v", issues)
	}
}
