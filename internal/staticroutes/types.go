// Package staticroutes parses and evaluates the in-repo routing rules of a
// static site: Netlify style _redirects and _headers files, the app.yaml
// routes and headers blocks, and the SPA, clean URL and 404 options.
package staticroutes

import "github.com/GLINCKER/levelrail/internal/spec"

// Rule and header sources, shown next to each parsed rule.
const (
	SourceRedirects = spec.StaticSourceRedirects
	SourceHeaders   = spec.StaticSourceHeaders
	SourceSpec      = spec.StaticSourceSpec
)

// Problem severities.
const (
	SeverityError   = spec.StaticSeverityError
	SeverityWarning = spec.StaticSeverityWarning
)

// Trailing slash modes for Options.TrailingSlash.
const (
	SlashOff    = spec.StaticSlashOff
	SlashAdd    = spec.StaticSlashAdd
	SlashRemove = spec.StaticSlashRemove
)

// SidecarSuffix names the compiled rules file stored beside a published tree.
const SidecarSuffix = spec.StaticSidecarSuffix

type (
	// QueryMatch requires a query parameter.
	QueryMatch = spec.StaticQueryMatch
	// Rule is one redirect, rewrite or status rule.
	Rule = spec.StaticRule
	// HeaderKV is one response header.
	HeaderKV = spec.StaticHeaderKV
	// HeaderBlock applies header changes to matching paths.
	HeaderBlock = spec.StaticHeaderBlock
	// Options are the static serving switches.
	Options = spec.StaticServeOptions
)

// Problem is one validation finding. Line is 0 for spec rules.
type Problem = spec.StaticProblem

// Ruleset is everything compiled for one deploy; Resolve evaluates it.
type Ruleset struct {
	spec.StaticRuleset
}

var placeholderRe = spec.StaticPlaceholderRe
