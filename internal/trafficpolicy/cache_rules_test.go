package trafficpolicy

import (
	"slices"
	"testing"
)

func TestCacheRuleOptions(t *testing.T) {
	l := DefaultLimits()
	base := func(r CacheRule) Cache {
		r.Match = Match{Kind: MatchPrefix, Path: "/a"}
		return Cache{Enabled: true, Rules: []CacheRule{r}}
	}
	tests := []struct {
		name string
		rule CacheRule
		ok   bool
	}{
		{"ttl only", CacheRule{TTLSeconds: 60}, true},
		{"never needs no ttl", CacheRule{Never: true}, true},
		{"stale if error", CacheRule{TTLSeconds: 60, StaleIfErrorSeconds: 600}, true},
		{"negative stale if error", CacheRule{TTLSeconds: 60, StaleIfErrorSeconds: -1}, false},
		{"ignored params", CacheRule{TTLSeconds: 60, IgnoreQueryParams: []string{"utm_source", "fbclid"}}, true},
		{"param with equals", CacheRule{TTLSeconds: 60, IgnoreQueryParams: []string{"a=b"}}, false},
		{"empty param", CacheRule{TTLSeconds: 60, IgnoreQueryParams: []string{""}}, false},
		{"key cookie needs opt in", CacheRule{TTLSeconds: 60, KeyCookies: []string{"lang"}}, false},
		{"key cookie with opt in", CacheRule{TTLSeconds: 60, CacheWithCookies: true, KeyCookies: []string{"lang"}}, true},
		{"bad cookie name", CacheRule{TTLSeconds: 60, CacheWithCookies: true, KeyCookies: []string{"a b"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base(tt.rule)
			if err := c.Validate(l); (err == nil) != tt.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestStaticAssetsPresetIsValid(t *testing.T) {
	c := Cache{Enabled: true, Rules: StaticAssetsPreset()}
	if err := c.Validate(DefaultLimits()); err != nil {
		t.Fatalf("preset does not validate: %v", err)
	}
	m := c.Rules[0].Match
	for path, want := range map[string]bool{"/app.js": true, "/img/Logo.PNG": true, "/api/users": false, "/page.html": false} {
		if got := m.Matches("GET", path); got != want {
			t.Errorf("preset matches %s = %v, want %v", path, got, want)
		}
	}
}

func TestPresets(t *testing.T) {
	tests := []struct {
		name      string
		wantVary  []string
		wantRules int
		path      string
		wantMatch int
	}{
		{PresetStaticAssets, nil, 1, "/app.js", 0},
		{PresetNextJS, NextJSVary(), 2, "/_next/static/chunks/a.js", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, ok := Presets()[tt.name]
			if !ok || len(rules) != tt.wantRules {
				t.Fatalf("Presets()[%q] has %d rules, want %d", tt.name, len(rules), tt.wantRules)
			}
			if err := (&Cache{Enabled: true, Rules: rules}).Validate(DefaultLimits()); err != nil {
				t.Fatalf("preset does not validate: %v", err)
			}
			if !rules[tt.wantMatch].Match.Matches("GET", tt.path) {
				t.Errorf("rule %d does not match %s", tt.wantMatch, tt.path)
			}
			if got := rules[len(rules)-1].Vary; !slices.Equal(got, tt.wantVary) {
				t.Errorf("last rule Vary = %v, want %v", got, tt.wantVary)
			}
		})
	}
}

func TestMatchesNextJSVary(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  bool
	}{
		{"full next set", []string{"Rsc", "Next-Router-State-Tree", "Next-Router-Prefetch"}, true},
		{"rsc only", []string{"rsc"}, true},
		{"accept language", []string{"Accept-Language"}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesNextJSVary(tt.names); got != tt.want {
				t.Fatalf("MatchesNextJSVary(%v) = %v, want %v", tt.names, got, tt.want)
			}
		})
	}
}
