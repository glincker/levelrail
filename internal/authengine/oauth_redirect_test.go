package authengine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestOAuthRedirectResolve(t *testing.T) {
	t.Setenv(EnvDashboardURL, "")
	cb := "/api/v1/auth/oauth/google/callback"
	tests := []struct {
		name      string
		dashboard string
		env       string
		host      string
		proto     string
		remote    string
		want      string
		wantErr   bool
	}{
		{"dashboard url ignores spoofed host", "https://dash.example.com/", "", "evil.test", "http", "203.0.113.9:1", "https://dash.example.com" + cb, false},
		{"env dashboard url used when setting empty", "", "https://env.example.com", "evil.test", "", "203.0.113.9:1", "https://env.example.com" + cb, false},
		{"untrusted peer without dashboard url is refused", "", "", "evil.test", "https", "203.0.113.9:1", "", true},
		{"loopback peer trusts host and proto", "", "", "dash.test", "https", "127.0.0.1:5", "https://dash.test" + cb, false},
		{"loopback peer plain http", "", "", "dash.test", "", "[::1]:5", "http://dash.test" + cb, false},
		{"bogus forwarded proto ignored", "", "", "dash.test", "javascript", "127.0.0.1:5", "http://dash.test" + cb, false},
		{"no host fails", "", "", "", "", "127.0.0.1:5", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvDashboardURL, tc.env)
			res := OAuthRedirectResolver{DashboardURL: func(context.Context) string { return tc.dashboard }}
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			r.Host = tc.host
			r.RemoteAddr = tc.remote
			if tc.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tc.proto)
			}
			got, err := res.Resolve(r, "google")
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got %q, %v; want %q (err %v)", got, err, tc.want, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrOAuthRedirectUnavailable) && tc.host != "" {
				t.Fatalf("err = %v, want ErrOAuthRedirectUnavailable", err)
			}
		})
	}
}

func TestOAuthRedirectResolve_CustomTrustedPeer(t *testing.T) {
	t.Setenv(EnvDashboardURL, "")
	res := OAuthRedirectResolver{TrustedPeer: func(*http.Request) bool { return true }}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Host = "proxy.test"
	r.RemoteAddr = "10.0.0.2:1"
	got, err := res.Resolve(r, "github")
	if err != nil || got != "http://proxy.test/api/v1/auth/oauth/github/callback" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestOAuthAllowedHosts(t *testing.T) {
	t.Setenv(EnvOAuthAllowedHosts, "")
	if got := oauthAllowedHosts(Config{BaseURL: "http://127.0.0.1:9100"}); got != nil {
		t.Fatalf("no dashboard URL must leave hosts unrestricted, got %v", got)
	}
	got := oauthAllowedHosts(Config{BaseURL: "http://127.0.0.1:9100", MFA: MFAConfig{DashboardURL: "https://dash.example.com"}})
	for _, want := range []string{"dash.example.com", "127.0.0.1:9100"} {
		if !slices.Contains(got, want) {
			t.Fatalf("hosts %v missing %q", got, want)
		}
	}
	t.Setenv(EnvOAuthAllowedHosts, "alt.example.com, other.example.com:8443")
	got = oauthAllowedHosts(Config{})
	if !slices.Contains(got, "alt.example.com") || !slices.Contains(got, "other.example.com:8443") {
		t.Fatalf("env hosts missing: %v", got)
	}
}
