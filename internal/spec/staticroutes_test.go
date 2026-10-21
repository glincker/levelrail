package spec

import (
	"strings"
	"testing"
)

func TestParseStaticRouting(t *testing.T) {
	const head = "version: 1\nservices:\n  web:\n    build: { type: static, path: dist }\n    domains: [a.example.com]\n"
	tests := []struct {
		name, body, wantErr string
	}{
		{"all fields", `    routes:
      - { from: /old/*, to: "/new/:splat", status: 301, force: true }
      - { from: /store, to: "/blog/:id", query: { id: ":id" } }
    headers:
      - { path: "/*", values: { X-Frame-Options: DENY }, remove: [Server] }
    static: { spa: true, cleanUrls: true, trailingSlash: remove, notFound: 404.html }
`, ""},
		{"unknown route key", "    routes:\n      - { from: /a, to: /b, bogus: 1 }\n", "bogus"},
		{"bad status", "    routes:\n      - { from: /a, to: /b, status: 500 }\n", "status"},
		{"open redirect target", "    routes:\n      - { from: /a, to: //evil.example.com }\n", "//"},
		{"header injection", "    headers:\n      - { path: /*, values: { X-A: \"a\\r\\nSet-Cookie: x=1\" } }\n", "single line"},
		{"bad slash mode", "    static: { trailingSlash: sideways }\n", "trailingSlash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(head + tc.body))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestStaticRoutingOnlyForStaticBuilds(t *testing.T) {
	_, err := Parse([]byte("version: 1\nservices:\n  web:\n    build: { type: dockerfile }\n    static: { spa: true }\n"))
	if err == nil || !strings.Contains(err.Error(), "only meaningful for build.type: static") {
		t.Fatalf("err = %v", err)
	}
}

func TestStaticRoutingInputIsDeterministic(t *testing.T) {
	s := Service{Headers: []HeaderRule{{Path: "/*", Values: map[string]string{"B-H": "1", "A-H": "2"}}}, Routes: []Route{{From: "/a", To: "/b", Query: map[string]string{"z": "1", "a": "2"}}}}
	in := s.StaticRoutingInput()
	if in.HeaderBlocks[0].Set[0].Name != "A-H" || in.Routes[0].Query[0].Key != "a" {
		t.Fatalf("input = %+v", in)
	}
}
