package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportDirConvertsAllShapes(t *testing.T) {
	dir := t.TempDir()
	w := func(n, s string) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w("_redirects", "/a /b 301\n/c /d Country=US 302\n")
	w("_headers", "/*\n  X-Frame-Options: DENY\n")
	w("site.toml", "[[redirects]]\nfrom = \"/old\"\nto = \"/new\"\nstatus = 302\n\n[[headers]]\nfor = \"/app/*\"\n  [headers.values]\n  Cache-Control = \"no-store\"\n")
	w("rules.json", `{"redirects":[{"source":"/x/:p*","destination":"/y/:p*","permanent":true},{"source":"/m","destination":"/n","has":[{"type":"host"}]}],"rewrites":[{"source":"/api","destination":"/index.html"}],"headers":[{"source":"/(.*)","headers":[{"key":"X-A","value":"1"}]}]}`)
	w("package.json", `{"name":"x"}`)

	res, err := ImportDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, h := res.Render()
	for _, want := range []string{"/a /b 301", "/old /new 302", "/x/* /y/:splat 308", "/api /index.html 200"} {
		if !strings.Contains(r, want) {
			t.Errorf("redirects missing %q in:\n%s", want, r)
		}
	}
	for _, want := range []string{"/*\n  X-Frame-Options: DENY", "/app/*\n  Cache-Control: no-store"} {
		if !strings.Contains(h, want) {
			t.Errorf("headers missing %q in:\n%s", want, h)
		}
	}
	if len(res.Unsupported) != 2 || len(res.Sources) != 4 {
		t.Fatalf("unsupported=%v sources=%v", res.Unsupported, res.Sources)
	}
}
