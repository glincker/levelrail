package staticroutes

import (
	"strings"
	"testing"
)

func TestParseRedirects(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		from    string
		to      string
		status  int
		force   bool
		query   int
		wantErr string
	}{
		{name: "default status", line: "/home /", from: "/home", to: "/", status: 301},
		{name: "explicit 302", line: "/old /new 302", from: "/old", to: "/new", status: 302},
		{name: "force", line: "/old /new 301!", from: "/old", to: "/new", status: 301, force: true},
		{name: "splat", line: "/blog/* /news/:splat", from: "/blog/*", to: "/news/:splat", status: 301},
		{name: "placeholders", line: "/news/:year/:month /blog/:year-:month", from: "/news/:year/:month", to: "/blog/:year-:month", status: 301},
		{name: "rewrite 200", line: "/* /index.html 200", from: "/*", to: "/index.html", status: 200},
		{name: "404 page", line: "/ecommerce/* /store-closed.html 404", from: "/ecommerce/*", to: "/store-closed.html", status: 404},
		{name: "410", line: "/gone /gone.html 410", from: "/gone", to: "/gone.html", status: 410},
		{name: "absolute target", line: "/docs https://docs.example.com 301", from: "/docs", to: "https://docs.example.com", status: 301},
		{name: "absolute with splat path", line: "/api/* https://api.example.com/:splat 307", from: "/api/*", to: "https://api.example.com/:splat", status: 307},
		{name: "query match", line: "/store id=:id /blog/:id 301", from: "/store", to: "/blog/:id", status: 301, query: 1},
		{name: "two query matches literal", line: "/s a=1 b=:b /r/:b 302", from: "/s", to: "/r/:b", status: 302, query: 2},
		{name: "extra spaces and tabs", line: "/a\t\t/b   301", from: "/a", to: "/b", status: 301},
		{name: "trailing slash from", line: "/a/ /b/", from: "/a/", to: "/b/", status: 301},
		{name: "query in destination", line: "/s /search?q=x", from: "/s", to: "/search?q=x", status: 301},

		{name: "missing destination", line: "/only", wantErr: "missing the destination"},
		{name: "from not a path", line: "example.com/a /b", wantErr: "must be a path"},
		{name: "host based rule", line: "https://old.example.com/* https://new.example.com/:splat 301", wantErr: "host based"},
		{name: "country condition", line: "/* /us/:splat 302 Country=us", wantErr: "not supported"},
		{name: "language condition without status", line: "/* /fr/:splat Language=fr", wantErr: "not supported"},
		{name: "bad status", line: "/a /b 999", wantErr: "not supported"},
		{name: "status 500", line: "/a /b 500", wantErr: "not supported"},
		{name: "junk token", line: "/a /b banana", wantErr: "unexpected"},
		{name: "protocol relative", line: "/a //evil.example.com 301", wantErr: "must not start with //"},
		{name: "javascript target", line: "/a javascript:alert(1) 301", wantErr: "path starting with /"},
		{name: "relative target", line: "/a other 301", wantErr: "path starting with /"},
		{name: "credentials in target", line: "/a https://user@evil.example.com 301", wantErr: "credentials"},
		{name: "placeholder host", line: "/:h/x https://:h.example.com/x 301", wantErr: "host of an absolute"},
		{name: "splat host", line: "/* https://*.example.com 301", wantErr: "host of an absolute"},
		{name: "unknown placeholder in to", line: "/a /b/:id", wantErr: "no such placeholder"},
		{name: "splat in the middle", line: "/a/*/b /c", wantErr: "last segment"},
		{name: "partial segment placeholder", line: "/a-:x /c", wantErr: "whole segment"},
		{name: "repeated placeholder", line: "/:a/:a /c", wantErr: "repeated"},
		{name: "self redirect", line: "/a /a 301", wantErr: "to itself"},
		{name: "external proxy rewrite", line: "/api/* https://api.example.com/:splat 200", wantErr: "proxy"},
		{name: "external 404", line: "/a https://x.example.com 404", wantErr: "needs a path"},
		{name: "backslash", line: `/a /b\c`, wantErr: "backslashes"},
		{name: "control char", line: "/a /b\x01c", wantErr: "control characters"},
		{name: "bad query match", line: "/s =x /b", wantErr: "must be key=value"},
		{name: "query placeholder repeats path", line: "/p/:id q=:id /x/:id", wantErr: "repeats"},
	}
	l := DefaultLimits()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rules, probs := ParseRedirects(tc.line, l)
			if tc.wantErr != "" {
				if len(rules) != 0 || len(probs) != 1 || !strings.Contains(probs[0].Message, tc.wantErr) {
					t.Fatalf("want one problem containing %q, got rules=%v probs=%v", tc.wantErr, rules, probs)
				}
				if probs[0].Line != 1 || probs[0].Severity != SeverityError {
					t.Fatalf("problem should be an error on line 1: %+v", probs[0])
				}
				return
			}
			if len(probs) != 0 || len(rules) != 1 {
				t.Fatalf("want one rule, got rules=%v probs=%v", rules, probs)
			}
			r := rules[0]
			if r.From != tc.from || r.To != tc.to || r.Status != tc.status || r.Force != tc.force || len(r.Query) != tc.query {
				t.Fatalf("got %+v", r)
			}
			if r.Source != "_redirects:1" {
				t.Fatalf("source = %q", r.Source)
			}
		})
	}
}

func TestParseRedirectsNetlifyDocSample(t *testing.T) {
	src := "# Redirect from a single page\r\n/home / 301\r\n\r\n" +
		"# Rewrite to a single page app\n/app/* /app/index.html 200\n" +
		"/news/:year/:month/:date/:slug /blog/:year/:month/:date/:slug\n" +
		"/store id=:id /blog/:id 301\n" +
		"/articles/* /posts/:splat 301!\n" +
		"/something /else 200 Role=admin\n" +
		"/ecommerce/* /store-closed.html 404\n"
	rules, probs := ParseRedirects(src, DefaultLimits())
	if len(rules) != 6 || len(probs) != 1 {
		t.Fatalf("rules=%d probs=%v", len(rules), probs)
	}
	if probs[0].Line != 9 || !strings.Contains(probs[0].Message, "Role=admin") {
		t.Fatalf("unexpected problem %+v", probs[0])
	}
	if rules[4].Source != "_redirects:8" || !rules[4].Force {
		t.Fatalf("sources or force wrong: %+v %+v", rules[4], rules[3])
	}
}

func TestParseRedirectsLongValueRejected(t *testing.T) {
	l := DefaultLimits()
	l.MaxValueLen = 20
	_, probs := ParseRedirects("/a /"+strings.Repeat("b", 30), l)
	if len(probs) != 1 || !strings.Contains(probs[0].Message, "at most 20 bytes") {
		t.Fatalf("probs = %v", probs)
	}
}

func TestParseRedirectsMatchesPatternsCaseInsensitive(t *testing.T) {
	rules, _ := ParseRedirects("/About /about-us", DefaultLimits())
	if rules[0].Pattern().Match("/about") == nil || rules[0].Pattern().Match("/ABOUT/") == nil {
		t.Fatal("matching should ignore case and a trailing slash")
	}
	if rules[0].Pattern().Match("/about/x") != nil {
		t.Fatal("exact path must not match a deeper one")
	}
}

func TestSubstituteEscapes(t *testing.T) {
	got := substitute("/n/:splat?x=:q", map[string]string{"splat": "a b/c\r\nd", "q": "1&2"})
	want := "/n/a%20b/c%0D%0Ad?x=1%262"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if guardLocation("//evil.com/x") != "/evil.com/x" || guardLocation(`/\evil.com`) != "/evil.com" {
		t.Fatal("guardLocation must collapse leading slashes")
	}
}
