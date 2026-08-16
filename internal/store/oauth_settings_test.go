package store

import (
	"context"
	"testing"
)

func TestGetOAuthProviderSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, provider := range []string{OAuthProviderGoogle, OAuthProviderGitHub} {
		got, err := db.GetOAuthProviderSettings(ctx, provider)
		if err != nil {
			t.Fatalf("GetOAuthProviderSettings(%q) error = %v", provider, err)
		}
		want := OAuthProviderSettings{Provider: provider}
		if got != want {
			t.Errorf("GetOAuthProviderSettings(%q) = %+v, want %+v", provider, got, want)
		}
	}
}

func TestUpdateAndGetOAuthProviderSettings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := OAuthProviderSettings{
		Provider:           OAuthProviderGoogle,
		Enabled:            true,
		ClientID:           "client-123.apps.googleusercontent.com",
		AllowedEmailDomain: "example.com",
	}
	if err := db.UpdateOAuthProviderSettings(ctx, want); err != nil {
		t.Fatalf("UpdateOAuthProviderSettings() error = %v", err)
	}

	got, err := db.GetOAuthProviderSettings(ctx, OAuthProviderGoogle)
	if err != nil {
		t.Fatalf("GetOAuthProviderSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetOAuthProviderSettings() = %+v, want %+v", got, want)
	}

	// The other provider's row must be untouched.
	github, err := db.GetOAuthProviderSettings(ctx, OAuthProviderGitHub)
	if err != nil {
		t.Fatalf("GetOAuthProviderSettings(github) error = %v", err)
	}
	if github.Enabled {
		t.Errorf("github settings changed by a google update: %+v", github)
	}
}

func TestListOAuthProviderSettings_BothProvidersSeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.ListOAuthProviderSettings(ctx)
	if err != nil {
		t.Fatalf("ListOAuthProviderSettings() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ListOAuthProviderSettings()) = %d, want 2", len(got))
	}
	if got[0].Provider != OAuthProviderGitHub || got[1].Provider != OAuthProviderGoogle {
		t.Errorf("ListOAuthProviderSettings() providers = [%q, %q], want alphabetical [github, google]", got[0].Provider, got[1].Provider)
	}
}

func TestOAuthProviderSecretsKey(t *testing.T) {
	if got, want := OAuthProviderSecretsKey("google"), "oauth-provider/google"; got != want {
		t.Errorf("OAuthProviderSecretsKey(google) = %q, want %q", got, want)
	}
}
