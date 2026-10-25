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
		"typo in domain":    {"version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    domains: [\"${{ env.HOST }}.example.com\"]\n", "unresolved placeholder"},
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
