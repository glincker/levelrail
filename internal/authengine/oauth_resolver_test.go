package authengine

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidOAuthProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"google", true}, {"github", true}, {"microsoft", true}, {"oidc", true},
		{"gitlab", true}, {"bitbucket", true}, {"discord", true}, {"slack", true},
		{"linkedin", true}, {"twitch", true}, {"facebook", true},
		{"x", false}, {"apple", false}, {"", false}, {"GitHub", false}, {"../etc", false},
	}
	for _, tc := range tests {
		if got := validOAuthProvider(tc.name); got != tc.want {
			t.Errorf("validOAuthProvider(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestProviderResolverBuild(t *testing.T) {
	t.Parallel()
	r := newProviderResolver(&oauthRuntime{})
	tests := []struct {
		provider  string
		wantHost  string
		wantScope string
	}{
		{store.OAuthProviderGoogle, "accounts.google.com", "openid"},
		{store.OAuthProviderGitHub, "github.com", "user"},
		{store.OAuthProviderMicrosoft, "login.microsoftonline.com", "openid"},
		{store.OAuthProviderGitLab, "gitlab.com", "openid"},
		{store.OAuthProviderBitbucket, "bitbucket.org", "email"},
		{store.OAuthProviderDiscord, "discord.com", "identify"},
		{store.OAuthProviderSlack, "slack.com", "openid"},
		{store.OAuthProviderLinkedIn, "linkedin.com", "openid"},
		{store.OAuthProviderTwitch, "id.twitch.tv", "user%3Aread%3Aemail"},
		{store.OAuthProviderFacebook, "facebook.com", "public_profile"},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			p, err := r.build(context.Background(), store.OAuthProviderSettings{Provider: tc.provider, ClientID: "id"}, "secret")
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if p.Name() != tc.provider {
				t.Errorf("Name() = %q, want %q", p.Name(), tc.provider)
			}
			u := p.AuthURL("state", "challenge", "https://lr.example/cb", nil)
			if !strings.Contains(u, tc.wantHost) || !strings.Contains(u, tc.wantScope) {
				t.Errorf("AuthURL = %q, want host %q and scope %q", u, tc.wantHost, tc.wantScope)
			}
		})
	}

	if _, err := r.build(context.Background(), store.OAuthProviderSettings{Provider: "x"}, "s"); err == nil {
		t.Error("unknown provider must not build")
	}
}

func TestGitLabBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       string
		insecure bool
		want     string
		wantErr  bool
	}{
		{"empty is gitlab.com", "", false, "", false},
		{"https trimmed", " https://git.example.com/ ", false, "https://git.example.com", false},
		{"http refused", "http://git.example.com", false, "", true},
		{"http allowed by flag", "http://git.example.com", true, "http://git.example.com", false},
		{"userinfo refused", "https://u:p@git.example.com", false, "", true},
		{"no host", "https://", false, "", true},
		{"query refused", "https://git.example.com?x=1", false, "", true},
		{"ftp refused", "ftp://git.example.com", true, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := gitlabBaseURL(tc.in, tc.insecure)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("gitlabBaseURL(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
			}
		})
	}
}
