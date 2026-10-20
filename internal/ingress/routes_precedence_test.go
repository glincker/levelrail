package ingress

import (
	"slices"
	"testing"
)

func TestBuildRoutesConfig_ExactHostBeatsWildcard(t *testing.T) {
	tests := []struct {
		name   string
		routes []ProxyRoute
		redir  []RedirectRoute
		want   []string
	}{
		{
			name:   "wildcard declared first still goes last",
			routes: []ProxyRoute{{Hosts: []string{"*.example.com"}, BackendDial: "a:1"}, {Hosts: []string{"api.example.com"}, BackendDial: "b:1"}},
			want:   []string{"api.example.com", "*.example.com"},
		},
		{
			name:   "relative order kept within each group",
			routes: []ProxyRoute{{Hosts: []string{"*.a.com"}, BackendDial: "a:1"}, {Hosts: []string{"x.com"}, BackendDial: "b:1"}, {Hosts: []string{"*.apps.a.com"}, BackendDial: "c:1"}, {Hosts: []string{"y.com"}, BackendDial: "d:1"}},
			want:   []string{"x.com", "y.com", "*.a.com", "*.apps.a.com"},
		},
		{
			name:   "wildcard redirect does not shadow an exact proxy",
			routes: []ProxyRoute{{Hosts: []string{"app.example.com"}, BackendDial: "a:1"}},
			redir:  []RedirectRoute{{Hosts: []string{"*.example.com"}, TargetURL: "https://example.com"}},
			want:   []string{"app.example.com", "*.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":443", Routes: tt.routes, RedirectRoutes: tt.redir})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range cfg.Apps.HTTP.Servers["s"].Routes {
				got = append(got, r.Match[0].Host[0])
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("route order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateWildcardDomain_Table(t *testing.T) {
	tests := []struct {
		domain string
		ok     bool
	}{
		{"*.example.com", true},
		{"*.apps.example.com", true},
		{"example.com", true},
		{"*.com", false},
		{"*.*.example.com", false},
		{"a.*.example.com", false},
		{"*example.com", false},
		{"*.", false},
	}
	for _, tt := range tests {
		if err := ValidateWildcardDomain(tt.domain); (err == nil) != tt.ok {
			t.Errorf("ValidateWildcardDomain(%q) = %v, want ok=%v", tt.domain, err, tt.ok)
		}
	}
}
