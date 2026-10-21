// Package staticroutes parses and evaluates the in-repo routing rules of a
// static site: Netlify style _redirects and _headers files, the app.yaml
// routes and headers blocks, and the SPA, clean URL and 404 options.
package staticroutes

import "regexp"

// Rule and header sources, shown next to each parsed rule.
const (
	SourceRedirects = "_redirects"
	SourceHeaders   = "_headers"
	SourceSpec      = "app.yaml"
)

// Problem severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Trailing slash modes for Options.TrailingSlash.
const (
	SlashOff    = "off"
	SlashAdd    = "add"
	SlashRemove = "remove"
)

// SidecarSuffix names the compiled rules file stored beside a published tree.
const SidecarSuffix = ".routes.json"

// QueryMatch requires a query parameter; Value is a literal or ":name".
type QueryMatch struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Rule is one redirect, rewrite or status rule.
type Rule struct {
	Source string       `json:"source"`
	From   string       `json:"from"`
	To     string       `json:"to"`
	Status int          `json:"status"`
	Force  bool         `json:"force,omitempty"`
	Query  []QueryMatch `json:"query,omitempty"`

	pat *pattern
}

// HeaderKV is one response header.
type HeaderKV struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HeaderBlock applies Set and Remove to responses whose path matches Path.
type HeaderBlock struct {
	Source string     `json:"source"`
	Path   string     `json:"path"`
	Set    []HeaderKV `json:"set,omitempty"`
	Remove []string   `json:"remove,omitempty"`

	pat *pattern
}

// Options are the static serving switches.
type Options struct {
	SPA           bool   `json:"spa,omitempty"`
	CleanURLs     bool   `json:"clean_urls,omitempty"`
	TrailingSlash string `json:"trailing_slash,omitempty"`
	NotFound      string `json:"not_found,omitempty"`
}

// Problem is one validation finding. Line is 0 for spec rules.
type Problem struct {
	Source   string `json:"source"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Ruleset is everything compiled for one deploy.
type Ruleset struct {
	Version  int           `json:"version"`
	Rules    []Rule        `json:"rules"`
	Headers  []HeaderBlock `json:"headers"`
	Options  Options       `json:"options"`
	Problems []Problem     `json:"problems"`
}

var placeholderRe = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
