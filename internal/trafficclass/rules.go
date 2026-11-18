package trafficclass

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultRulesMax caps path rules per app when APP_TRAFFIC_RULES_MAX is unset.
const DefaultRulesMax = 50

// RulesMaxFromEnv reads APP_TRAFFIC_RULES_MAX.
func RulesMaxFromEnv() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_TRAFFIC_RULES_MAX"))); err == nil && n > 0 {
		return n
	}
	return DefaultRulesMax
}

// Rule maps a path glob to a class. Only page, api, asset and ignored are valid targets.
// In Pattern, "*" matches any run of characters including "/"; a pattern with no "*" matches exactly.
type Rule struct {
	Pattern string
	Class   Class
}

// Rules is an ordered, immutable list of path rules; the first match wins.
type Rules struct {
	list []Rule
}

// NewRules validates rs against the per-app cap and returns them as a rule set.
func NewRules(rs []Rule) (*Rules, error) {
	if limit := RulesMaxFromEnv(); len(rs) > limit {
		return nil, fmt.Errorf("trafficclass: %d path rules exceed the limit of %d", len(rs), limit)
	}
	for _, r := range rs {
		if !strings.HasPrefix(r.Pattern, "/") {
			return nil, fmt.Errorf("trafficclass: path rule %q must start with /", r.Pattern)
		}
		switch r.Class {
		case Page, API, Asset, Ignored:
		default:
			return nil, fmt.Errorf("trafficclass: path rule %q targets %q, want page, api, asset or ignored", r.Pattern, r.Class)
		}
	}
	return &Rules{list: append([]Rule(nil), rs...)}, nil
}

// List returns a copy of the rules in order.
func (r *Rules) List() []Rule {
	if r == nil {
		return nil
	}
	return append([]Rule(nil), r.list...)
}

var defaultRules = mustRules([]Rule{
	{"/api/*", API}, {"/graphql", API}, {"/trpc/*", API}, {"/_next/data/*", API}, {"/socket.io/*", API},
	{"/_next/static/*", Asset}, {"/assets/*", Asset}, {"/static/*", Asset}, {"/build/*", Asset},
	{"/favicon.ico", Asset}, {"/robots.txt", Asset}, {"/sitemap*.xml", Asset}, {"/manifest.json", Asset},
	{"/.well-known/*", Asset},
})

func mustRules(rs []Rule) *Rules { return &Rules{list: rs} }

// DefaultRules returns the built-in rules the spec ships; callers replace them per app.
func DefaultRules() *Rules { return defaultRules }

func (r *Rules) match(path string) (idx int, c Class, ok bool) {
	for i, rule := range r.list {
		if globMatch(rule.Pattern, path) {
			return i, rule.Class, true
		}
	}
	return 0, Other, false
}

// globMatch matches s against pat where "*" spans any characters. It never allocates.
func globMatch(pat, s string) bool {
	px, sx, star, mark := 0, 0, -1, 0
	for sx < len(s) {
		switch {
		case px < len(pat) && pat[px] == '*':
			star, mark = px, sx
			px++
		case px < len(pat) && pat[px] == s[sx]:
			px++
			sx++
		case star >= 0:
			mark++
			sx, px = mark, star+1
		default:
			return false
		}
	}
	for px < len(pat) && pat[px] == '*' {
		px++
	}
	return px == len(pat)
}
