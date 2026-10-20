package trafficpolicy

import "testing"

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
