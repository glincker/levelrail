package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeGitHubAppPublic(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   *bool
	}{
		{name: "public app answers 200", status: http.StatusOK, want: ptrBool(true)},
		{name: "private app answers 404", status: http.StatusNotFound, want: ptrBool(false)},
		{name: "rate limited is unknown", status: http.StatusForbidden},
		{name: "server error is unknown", status: http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/apps/levelrail-test" {
					t.Errorf("path = %q", r.URL.Path)
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			got := probeGitHubAppPublic(context.Background(), srv.URL, "levelrail-test")
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGitHubAppMakePublicURL(t *testing.T) {
	tests := []struct {
		name, ownerType, owner, want string
	}{
		{name: "personal owner", ownerType: "User", owner: "someone", want: "https://github.com/settings/apps/my-app/advanced"},
		{name: "org owner", ownerType: "Organization", owner: "acme", want: "https://github.com/organizations/acme/settings/apps/my-app/advanced"},
		{name: "no installation yet", want: "https://github.com/settings/apps/my-app/advanced"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := githubAppMakePublicURL("https://github.com/", "my-app", tc.ownerType, tc.owner); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitHubAppAPIBase(t *testing.T) {
	if got := githubAppAPIBase("https://github.com"); got != "https://api.github.com" {
		t.Errorf("github.com base = %q", got)
	}
	if got := githubAppAPIBase("https://ghe.example.com/"); got != "https://ghe.example.com/api/v3" {
		t.Errorf("ghes base = %q", got)
	}
}

func ptrBool(b bool) *bool { return &b }
