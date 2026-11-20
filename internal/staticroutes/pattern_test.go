package staticroutes

import (
	"net/url"
	"testing"
)

// Every clean URL and trailing slash combination must settle: following a
// redirect at most twice ends on a served page or a 404, never a loop.
func TestCleanURLAndSlashMatrixSettles(t *testing.T) {
	paths := []string{"/", "/about", "/about/", "/about.html", "/docs", "/docs/", "/docs/index.html", "/docs/intro", "/docs/intro/", "/docs/intro.html", "/app.js", "/app.js/", "/nope", "/empty/"}
	for _, clean := range []bool{false, true} {
		for _, slash := range []string{"", SlashAdd, SlashRemove} {
			opts := Options{CleanURLs: clean, TrailingSlash: slash}
			rs := compile(t, Input{Options: opts})
			for _, p := range paths {
				path, query := p, ""
				for hop := 0; ; hop++ {
					d := rs.Resolve(site(), Request{Method: "GET", Path: path, RawQuery: query})
					if d.Kind != KindRedirect {
						if d.Kind != KindServe && d.Kind != KindNotFound {
							t.Fatalf("clean=%v slash=%q %s: unexpected kind %s", clean, slash, p, d.Kind)
						}
						break
					}
					if hop >= 2 {
						t.Fatalf("clean=%v slash=%q %s: redirect chain did not settle, last %s", clean, slash, p, d.Location)
					}
					u, err := url.Parse(d.Location)
					if err != nil {
						t.Fatal(err)
					}
					path, query = u.Path, u.RawQuery
				}
			}
		}
	}
}

func TestSlashRedirectsKeepQuery(t *testing.T) {
	rs := compile(t, Input{Options: Options{CleanURLs: true, TrailingSlash: SlashAdd}})
	d := rs.Resolve(site(), Request{Method: "GET", Path: "/about", RawQuery: "a=1"})
	if d.Kind != KindRedirect || d.Location != "/about/?a=1" {
		t.Fatalf("got kind=%s loc=%q", d.Kind, d.Location)
	}
}
