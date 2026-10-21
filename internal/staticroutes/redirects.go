package staticroutes

import (
	"github.com/GLINCKER/levelrail/internal/spec"
)

func isRedirectStatus(s int) bool { return spec.IsStaticRedirectStatus(s) }

// ParseRedirects parses Netlify _redirects syntax; bad lines are reported, never applied.
func ParseRedirects(src string, l Limits) ([]Rule, []Problem) {
	return spec.ParseStaticRedirects(src, l)
}

// ValidateRule checks a rule and prepares its matcher. It is the single
// gate for file rules and app.yaml rules alike.
func ValidateRule(r *Rule, l Limits) error { return spec.ValidateStaticRule(r, l) }
