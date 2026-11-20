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

// WithoutProxy returns a copy of rs with every proxy rule dropped, for hosts
// that must serve files only (per-deploy preview hosts).
func (rs *Ruleset) WithoutProxy() *Ruleset {
	if rs == nil || rs.ProxyRuleCount() == 0 {
		return rs
	}
	out := *rs
	out.Rules = make([]Rule, 0, len(rs.Rules))
	for _, r := range rs.Rules {
		if !r.IsProxy() {
			out.Rules = append(out.Rules, r)
		}
	}
	return &out
}
