package samlsso

import "testing"

func TestResolveRole(t *testing.T) {
	mappings := []GroupRole{
		{Group: "Engineering", RoleID: "role_operator", Priority: 10},
		{Group: "Admins", RoleID: "role_admin", Priority: 100},
		{Group: "everyone", RoleID: "role_viewer", Priority: 0},
		{Group: "alpha", RoleID: "role_custom_a", Priority: 10},
	}
	for _, tc := range []struct {
		name   string
		groups []string
		want   string
		ok     bool
	}{
		{"no groups", nil, "", false},
		{"no match", []string{"sales"}, "", false},
		{"case insensitive", []string{"engineering"}, "role_operator", true},
		{"highest priority wins", []string{"everyone", "Admins", "Engineering"}, "role_admin", true},
		{"tie breaks alphabetically", []string{"Engineering", "alpha"}, "role_custom_a", true},
		{"low priority alone", []string{"everyone"}, "role_viewer", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ResolveRole(tc.groups, mappings)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ResolveRole = %q %v, want %q %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	for _, tc := range []struct {
		email   string
		domains []string
		want    bool
	}{
		{"a@example.com", nil, true},
		{"a@example.com", []string{"example.com"}, true},
		{"a@EXAMPLE.com", []string{"@example.com"}, true},
		{"a@evil.com", []string{"example.com"}, false},
		{"a@sub.example.com", []string{"example.com"}, false},
		{"no-at", []string{"example.com"}, false},
	} {
		if got := EmailDomainAllowed(tc.email, tc.domains); got != tc.want {
			t.Errorf("EmailDomainAllowed(%q, %v) = %v", tc.email, tc.domains, got)
		}
	}
}
