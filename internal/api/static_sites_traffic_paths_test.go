package api

import "testing"

func TestClassifyTrafficPath(t *testing.T) {
	cases := []struct {
		in    string
		kind  trafficPathKind
		label string
	}{
		{"/assets/index-a1b2c3.js", trafficPathAsset, "/assets/index-a1b2c3.js"},
		{"/about", trafficPathPage, "/about"},
		{"/About?x=1", trafficPathPage, "/About"},
		{"/blog/", trafficPathPage, "/blog"},
		{"/index.html", trafficPathPage, "/"},
		{"/docs/index.htm", trafficPathPage, "/docs"},
		{"/.well-known/x", trafficPathExcluded, "/.well-known/x"},
		{"/.env", trafficPathExcluded, "/.env"},
		{"/_next/static/a.js", trafficPathAsset, "/_next/static/a.js"},
		{"/favicon.ico", trafficPathAsset, "/favicon.ico"},
		{"/sitemap-0.xml", trafficPathAsset, "/sitemap-0.xml"},
		{"/site.css", trafficPathAsset, "/site.css"},
	}
	for _, c := range cases {
		kind, label, _ := classifyTrafficPath(c.in)
		if kind != c.kind || label != c.label {
			t.Errorf("%s = %d %q, want %d %q", c.in, kind, label, c.kind, c.label)
		}
	}
}

func TestIsProbePath(t *testing.T) {
	for in, want := range map[string]bool{
		"/wp-login.php": true, "/xmlrpc.php": true, "/.git/config": true, "/phpmyadmin/": true,
		"/admin": true, "/x.asp": true, "/.well-known/x": true,
		"/missing": false, "/blog/old-post": false, "/img/logo.png": false,
	} {
		if got := isProbePath(in); got != want {
			t.Errorf("%s = %v, want %v", in, got, want)
		}
	}
}
