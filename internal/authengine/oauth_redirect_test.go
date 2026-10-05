package authengine

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestOAuthRedirectURI(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		proto   string
		want    string
		wantErr bool
	}{
		{"plain http", "dash.test", "", "http://dash.test/api/v1/auth/oauth/google/callback", false},
		{"forwarded proto wins", "dash.test", "https", "https://dash.test/api/v1/auth/oauth/google/callback", false},
		{"no host fails", "", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			r.Host = tc.host
			if tc.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tc.proto)
			}
			got, err := oauthRedirectURI(r, "google")
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got %q, %v; want %q (err %v)", got, err, tc.want, tc.wantErr)
			}
		})
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
