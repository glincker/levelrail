package api

import (
	"path"
	"strings"
)

type trafficPathKind uint8

const (
	trafficPathPage trafficPathKind = iota
	trafficPathAsset
	trafficPathExcluded
)

// probePathFragments are substrings that mark scanner traffic rather than a
// real link on a static site.
var probePathFragments = []string{
	"/wp-", "/xmlrpc", "/phpmyadmin", "/pma", "/.env", "/.git", "/.svn", "/.aws",
	"/cgi-bin", "/boaform", "/actuator", "/administrator", "/manager/html",
	"/shell", "/eval-stdin", "/vendor/phpunit",
}

var probePathSuffixes = []string{".php", ".asp", ".aspx", ".jsp", ".cgi"}

var wellKnownNonPages = []string{"favicon", "robots.txt", "manifest", "apple-touch-icon", "browserconfig"}

// normalizeTrafficPath strips query, fragment, trailing slash and index.html.
// The label keeps its case; key is the lowercase form used for merging.
func normalizeTrafficPath(raw string) (label, key string) {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" || raw[0] != '/' {
		raw = "/" + raw
	}
	lower := strings.ToLower(raw)
	for _, idx := range []string{"/index.html", "/index.htm"} {
		if strings.HasSuffix(lower, idx) {
			raw = raw[:len(raw)-len(idx)]
			break
		}
	}
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		raw = "/"
	}
	return raw, strings.ToLower(raw)
}

// classifyTrafficPath sorts a request path into page, asset or excluded
// (dotfiles and dot-directories) and returns the normalised label and key.
func classifyTrafficPath(raw string) (trafficPathKind, string, string) {
	label, key := normalizeTrafficPath(raw)
	for _, seg := range strings.Split(key, "/") {
		if strings.HasPrefix(seg, ".") {
			return trafficPathExcluded, label, key
		}
	}
	if strings.HasPrefix(key, "/_") {
		return trafficPathAsset, label, key
	}
	base := path.Base(key)
	for _, w := range wellKnownNonPages {
		if strings.HasPrefix(base, w) {
			return trafficPathAsset, label, key
		}
	}
	if strings.HasPrefix(base, "sitemap") && strings.HasSuffix(base, ".xml") {
		return trafficPathAsset, label, key
	}
	switch path.Ext(base) {
	case "", ".html", ".htm":
		return trafficPathPage, label, key
	}
	return trafficPathAsset, label, key
}

// isProbePath reports whether a request path looks like a vulnerability scan.
func isProbePath(raw string) bool {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	p := strings.ToLower(raw)
	for _, f := range probePathFragments {
		if strings.Contains(p, f) {
			return true
		}
	}
	for _, s := range probePathSuffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	if p == "/admin" || strings.HasPrefix(p, "/admin/") {
		return true
	}
	// Dot-directories such as /.well-known are noise, not site links.
	kind, _, _ := classifyTrafficPath(raw)
	return kind == trafficPathExcluded
}
