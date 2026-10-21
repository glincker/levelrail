package staticroutes

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type mapStat struct{ fstest.MapFS }

func (m mapStat) Stat(name string) (fs.FileInfo, error) { return fs.Stat(m.MapFS, name) }

func site() mapStat {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return mapStat{fstest.MapFS{
		"index.html":       f("home"),
		"about.html":       f("about"),
		"404.html":         f("missing"),
		"app.js":           f("js"),
		"docs/index.html":  f("docs"),
		"docs/intro.html":  f("intro"),
		"empty/readme.txt": f("r"),
		"_redirects":       f("secret"),
		"_headers":         f("secret"),
	}}
}

func compile(t *testing.T, in Input) *Ruleset {
	t.Helper()
	rs, err := Compile(in, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		path     string
		query    string
		kind     string
		status   int
		file     string
		location string
	}{
		{name: "root index", path: "/", kind: KindServe, status: 200, file: "index.html"},
		{name: "exact file", path: "/about.html", kind: KindServe, status: 200, file: "about.html"},
		{name: "dir index with slash", path: "/docs/", kind: KindServe, status: 200, file: "docs/index.html"},
		{name: "dir without slash redirects", path: "/docs", kind: KindRedirect, status: 308, location: "/docs/"},
		{name: "dir redirect keeps query", path: "/docs", query: "a=1", kind: KindRedirect, status: 308, location: "/docs/?a=1"},
		{name: "missing uses 404.html", path: "/nope", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "path traversal cleaned", path: "/docs/../../etc/passwd", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "rules file never served", path: "/_redirects", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "headers file never served", path: "/_headers", kind: KindNotFound, status: 404, file: "404.html"},

		{name: "redirect rule", in: Input{Redirects: "/old /about.html 301"}, path: "/old", kind: KindRedirect, status: 301, location: "/about.html"},
		{name: "redirect trailing slash request", in: Input{Redirects: "/old /about.html 301"}, path: "/old/", kind: KindRedirect, status: 301, location: "/about.html"},
		{name: "redirect splat", in: Input{Redirects: "/blog/* /posts/:splat"}, path: "/blog/2024/hello", kind: KindRedirect, status: 301, location: "/posts/2024/hello"},
		{name: "redirect bare splat prefix", in: Input{Redirects: "/blog/* /posts/:splat"}, path: "/blog", kind: KindRedirect, status: 301, location: "/posts/"},
		{name: "redirect placeholders", in: Input{Redirects: "/n/:y/:m /a/:y-:m 302"}, path: "/n/2024/05", kind: KindRedirect, status: 302, location: "/a/2024-05"},
		{name: "redirect keeps query", in: Input{Redirects: "/old /new"}, path: "/old", query: "ref=x", kind: KindRedirect, status: 301, location: "/new?ref=x"},
		{name: "redirect to absolute", in: Input{Redirects: "/ext https://docs.example.com"}, path: "/ext", kind: KindRedirect, status: 301, location: "https://docs.example.com"},
		{name: "case insensitive", in: Input{Redirects: "/Old /new"}, path: "/oLD", kind: KindRedirect, status: 301, location: "/new"},
		{name: "query match", in: Input{Redirects: "/store id=:id /blog/:id 301"}, path: "/store", query: "id=42", kind: KindRedirect, status: 301, location: "/blog/42?id=42"},
		{name: "query match absent falls through", in: Input{Redirects: "/store id=:id /blog/:id 301"}, path: "/store", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "query literal mismatch", in: Input{Redirects: "/s a=1 /x"}, path: "/s", query: "a=2", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "open redirect via splat stays on host", in: Input{Redirects: "/go/* /:splat"}, path: "/go//evil.example.com", kind: KindRedirect, status: 301, location: "/evil.example.com"},
		{name: "encoded newline escaped", in: Input{Redirects: "/go/* /x/:splat"}, path: "/go/a\r\nSet-Cookie: x", kind: KindRedirect, status: 301, location: "/x/a%0D%0ASet-Cookie:%20x"},
		{name: "self redirect skipped", in: Input{Redirects: "/* /index.html 301"}, path: "/index.html", kind: KindServe, status: 200, file: "index.html"},

		{name: "unforced rule shadowed by file", in: Input{Redirects: "/about.html /elsewhere"}, path: "/about.html", kind: KindServe, status: 200, file: "about.html"},
		{name: "forced rule beats file", in: Input{Redirects: "/about.html /elsewhere 301!"}, path: "/about.html", kind: KindRedirect, status: 301, location: "/elsewhere"},
		{name: "first matching rule wins", in: Input{Redirects: "/a /one\n/a /two"}, path: "/a", kind: KindRedirect, status: 301, location: "/one"},
		{name: "spec rules rank before file rules", in: Input{
			Routes:    []Rule{{From: "/a", To: "/spec", Status: 302}},
			Redirects: "/a /file",
		}, path: "/a", kind: KindRedirect, status: 302, location: "/spec"},

		{name: "rewrite 200 serves target file", in: Input{Redirects: "/team /about.html 200"}, path: "/team", kind: KindServe, status: 200, file: "about.html"},
		{name: "rewrite 200 missing target falls through", in: Input{Redirects: "/team /gone.html 200"}, path: "/team", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "404 rule serves page with 404", in: Input{Redirects: "/old/* /about.html 404"}, path: "/old/x", kind: KindNotFound, status: 404, file: "about.html"},
		{name: "410 rule", in: Input{Redirects: "/gone /about.html 410"}, path: "/gone", kind: KindNotFound, status: 410, file: "about.html"},

		{name: "spa fallback", in: Input{Options: Options{SPA: true}}, path: "/dashboard/settings", kind: KindServe, status: 200, file: "index.html"},
		{name: "spa leaves missing asset as 404", in: Input{Options: Options{SPA: true}}, path: "/missing.js", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "spa real file wins", in: Input{Options: Options{SPA: true}}, path: "/app.js", kind: KindServe, status: 200, file: "app.js"},
		{name: "spa not for POST", in: Input{Options: Options{SPA: true}}, path: "/x", kind: KindNotFound, status: 404, file: "404.html"},
		{name: "custom not found", in: Input{Options: Options{NotFound: "about.html"}}, path: "/zzz", kind: KindNotFound, status: 404, file: "about.html"},

		{name: "clean url serves html", in: Input{Options: Options{CleanURLs: true}}, path: "/about", kind: KindServe, status: 200, file: "about.html"},
		{name: "clean url nested", in: Input{Options: Options{CleanURLs: true}}, path: "/docs/intro", kind: KindServe, status: 200, file: "docs/intro.html"},
		{name: "clean url redirects .html", in: Input{Options: Options{CleanURLs: true}}, path: "/about.html", kind: KindRedirect, status: 301, location: "/about"},
		{name: "clean url redirects index.html", in: Input{Options: Options{CleanURLs: true}}, path: "/docs/index.html", kind: KindRedirect, status: 301, location: "/docs/"},
		{name: "clean url root index", in: Input{Options: Options{CleanURLs: true}}, path: "/index.html", kind: KindRedirect, status: 301, location: "/"},
		{name: "clean url dir served directly", in: Input{Options: Options{CleanURLs: true}}, path: "/docs", kind: KindServe, status: 200, file: "docs/index.html"},
		{name: "404.html not redirected", in: Input{Options: Options{CleanURLs: true}}, path: "/404.html", kind: KindServe, status: 200, file: "404.html"},

		{name: "slash remove", in: Input{Options: Options{CleanURLs: true, TrailingSlash: SlashRemove}}, path: "/docs/", kind: KindRedirect, status: 308, location: "/docs"},
		{name: "slash remove serves bare dir", in: Input{Options: Options{TrailingSlash: SlashRemove}}, path: "/docs", kind: KindServe, status: 200, file: "docs/index.html"},
		{name: "slash add on clean file", in: Input{Options: Options{CleanURLs: true, TrailingSlash: SlashAdd}}, path: "/about", kind: KindRedirect, status: 308, location: "/about/"},
		{name: "slash add leaves real file alone", in: Input{Options: Options{TrailingSlash: SlashAdd}}, path: "/app.js", kind: KindServe, status: 200, file: "app.js"},
		{name: "slash add on dir", in: Input{Options: Options{TrailingSlash: SlashAdd}}, path: "/docs", kind: KindRedirect, status: 308, location: "/docs/"},
		{name: "dir without index is 404", path: "/empty/", kind: KindNotFound, status: 404, file: "404.html"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rs := compile(t, tc.in)
			method := "GET"
			if tc.name == "spa not for POST" {
				method = "POST"
			}
			d := rs.Resolve(site(), Request{Method: method, Path: tc.path, RawQuery: tc.query})
			if d.Kind != tc.kind || d.Status != tc.status || d.File != tc.file || d.Location != tc.location {
				t.Fatalf("got kind=%s status=%d file=%q loc=%q trace=%v", d.Kind, d.Status, d.File, d.Location, d.Trace)
			}
			if len(d.Trace) == 0 {
				t.Fatal("trace should explain the decision")
			}
		})
	}
}

func TestResolveNoNotFoundPageGivesPlain404(t *testing.T) {
	rs := compile(t, Input{})
	empty := mapStat{fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}}
	d := rs.Resolve(empty, Request{Method: "GET", Path: "/zzz"})
	if d.Kind != KindNotFound || d.Status != 404 || d.File != "" {
		t.Fatalf("got %+v", d)
	}
}

func TestResolveHeaders(t *testing.T) {
	rs := compile(t, Input{
		Headers: "/*\n  X-Frame-Options: DENY\n  Server: levelrail\n/static/*\n  Cache-Control: max-age=60\n  ! Server\n/about.html\n  Link: </a>\n  Link: </b>\n",
	})
	d := rs.Resolve(site(), Request{Method: "GET", Path: "/about.html"})
	got := map[string]string{}
	for _, h := range d.Headers {
		got[h.Name] = h.Value
	}
	if got["X-Frame-Options"] != "DENY" || got["Link"] != "</a>, </b>" || got["Cache-Control"] != "" {
		t.Fatalf("headers = %v", got)
	}
	d = rs.Resolve(site(), Request{Method: "GET", Path: "/static/app.js"})
	if len(d.Remove) != 1 || d.Remove[0] != "Server" {
		t.Fatalf("remove = %v", d.Remove)
	}
	redirect := compile(t, Input{Redirects: "/old /new", Headers: "/old\n  X-A: b\n"}).Resolve(site(), Request{Method: "GET", Path: "/old"})
	if redirect.Kind != KindRedirect || len(redirect.Headers) != 1 {
		t.Fatalf("headers should also apply to a redirect: %+v", redirect)
	}
}

func TestResolveLongPath(t *testing.T) {
	d := compile(t, Input{}).Resolve(site(), Request{Method: "GET", Path: "/" + strings.Repeat("a", 3000)})
	if d.Status != 414 {
		t.Fatalf("status = %d", d.Status)
	}
}
