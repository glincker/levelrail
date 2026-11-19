package staticroutes

import (
	"io/fs"
	"strings"
	"testing"
)

type noFiles struct{}

func (noFiles) Stat(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }

func TestProxyRuleParsesAndResolves(t *testing.T) {
	rules, probs := ParseRedirects("/api/* app://my-api/v1/:splat 200\n/health app://my-api/healthz 200\n", DefaultLimits())
	if len(probs) != 0 || len(rules) != 2 {
		t.Fatalf("rules=%d probs=%v", len(rules), probs)
	}
	if !rules[0].IsProxy() {
		t.Fatal("expected a proxy rule")
	}
	app, tmpl := rules[0].ProxyTarget()
	if app != "my-api" || tmpl != "/v1/:splat" {
		t.Fatalf("target %q %q", app, tmpl)
	}
	rs := &Ruleset{}
	rs.Rules = rules
	d := rs.Resolve(noFiles{}, Request{Method: "POST", Path: "/api/users/7"})
	if d.Kind != KindProxy || d.Location != "/v1/users/7" {
		t.Fatalf("decision %+v", d)
	}
	if d := rs.Resolve(noFiles{}, Request{Method: "GET", Path: "/health"}); d.Kind != KindProxy || d.Location != "/healthz" {
		t.Fatalf("exact rule: %+v", d)
	}
}

func TestProxyRuleRejectsUnsafeTargets(t *testing.T) {
	bad := map[string]string{
		"/a/* https://evil.example/:splat 200": "not supported",
		"/a/* app://x/y 301":                   "status 200",
		"/a/:id app://x/:id 200":               "literal segments",
		"/a/* app://x/../y 200":                "dot segments",
		"/a/* app://x/y#f 200":                 "dot segments",
		"/a/* app://x/:other 200":              "only :splat",
		"/a/b app://x/:splat 200":              "trailing *",
		"/a/* app://-bad/y 200":                "must name an app",
		"/a/* app:///y 200":                    "must name an app",
	}
	for line, want := range bad {
		_, probs := ParseRedirects(line, DefaultLimits())
		if len(probs) != 1 || !strings.Contains(probs[0].Message, want) {
			t.Fatalf("%q: want %q got %v", line, want, probs)
		}
	}
}

func TestProxyRuleCapAndPreview(t *testing.T) {
	l := DefaultLimits()
	l.MaxProxyRules = 1
	_, err := Compile(Input{Redirects: "/a/* app://x/:splat 200\n/b/* app://y/:splat 200\n"}, l)
	if err == nil || !strings.Contains(err.Error(), "APP_PAGES_PROXY_MAX_RULES") {
		t.Fatalf("cap not enforced: %v", err)
	}
	rs, err := Compile(Input{Redirects: "/a/* app://x/:splat 200\n/old /new 301\n"}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got := rs.WithoutProxy(); len(got.Rules) != 1 || got.Rules[0].IsProxy() || len(rs.Rules) != 2 {
		t.Fatalf("preview rules: %d, original %d", len(got.Rules), len(rs.Rules))
	}
	_, _, probs := CompileUI([]UIRedirect{{From: "/a/*", To: "app://x/:splat", Status: 200}}, nil, DefaultLimits())
	if len(probs) != 0 {
		t.Fatalf("ui proxy rule: %v", probs)
	}
}
