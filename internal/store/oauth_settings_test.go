package store

import (
	"context"
	"reflect"
	"testing"
)

// withoutPolicy drops the SSO policy so client settings compare with ==.
func withoutPolicy(s OAuthProviderSettings) OAuthProviderSettings {
	s.SSOPolicy = OAuthSSOPolicy{}
	return s
}

func TestGetOAuthProviderSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, provider := range []string{OAuthProviderGoogle, OAuthProviderGitHub, OAuthProviderOIDC} {
		got, err := db.GetOAuthProviderSettings(ctx, provider)
		if err != nil {
			t.Fatalf("GetOAuthProviderSettings(%q) error = %v", provider, err)
		}
		want := OAuthProviderSettings{Provider: provider}
		if !reflect.DeepEqual(withoutPolicy(got), want) {
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
	if !reflect.DeepEqual(withoutPolicy(got), want) {
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

func TestListOAuthProviderSettings_AllProvidersSeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.ListOAuthProviderSettings(ctx)
	if err != nil {
		t.Fatalf("ListOAuthProviderSettings() error = %v", err)
	}
	want := []string{
		OAuthProviderBitbucket, OAuthProviderCodeberg, OAuthProviderCognito, OAuthProviderDiscord,
		OAuthProviderDropbox, OAuthProviderFacebook, OAuthProviderFigma, OAuthProviderGitHub, OAuthProviderGitLab,
		OAuthProviderGoogle, OAuthProviderHuggingFace, OAuthProviderKakao, OAuthProviderLinear, OAuthProviderLinkedIn,
		OAuthProviderMicrosoft, OAuthProviderOIDC, OAuthProviderPatreon, OAuthProviderSalesforce, OAuthProviderSlack,
		OAuthProviderTwitch, OAuthProviderVercel,
	}
	if len(got) != len(want) {
		t.Fatalf("len(ListOAuthProviderSettings()) = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Provider != w {
			t.Errorf("ListOAuthProviderSettings()[%d].Provider = %q, want %q", i, got[i].Provider, w)
		}
	}
}

func TestUpdateAndGetOAuthProviderSettings_OIDCFields(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := OAuthProviderSettings{
		Provider:    OAuthProviderOIDC,
		Enabled:     true,
		ClientID:    "levelrail",
		IssuerURL:   "https://idp.example.com",
		DisplayName: "Okta",
	}
	if err := db.UpdateOAuthProviderSettings(ctx, want); err != nil {
		t.Fatalf("UpdateOAuthProviderSettings() error = %v", err)
	}

	got, err := db.GetOAuthProviderSettings(ctx, OAuthProviderOIDC)
	if err != nil {
		t.Fatalf("GetOAuthProviderSettings() error = %v", err)
	}
	if !reflect.DeepEqual(withoutPolicy(got), want) {
		t.Errorf("GetOAuthProviderSettings() = %+v, want %+v", got, want)
	}
}

func TestOAuthProviderSecretsKey(t *testing.T) {
	if got, want := OAuthProviderSecretsKey("google"), "oauth-provider/google"; got != want {
		t.Errorf("OAuthProviderSecretsKey(google) = %q, want %q", got, want)
	}
}

func TestOAuthSSOPolicy_DefaultsAreLeastPrivilege(t *testing.T) {
	db := openTestDB(t)
	got, err := db.GetOAuthProviderSettings(context.Background(), OAuthProviderOIDC)
	if err != nil {
		t.Fatal(err)
	}
	want := OAuthSSOPolicy{GroupsClaim: "groups", DefaultRoleID: "role_viewer", AllowedDomains: []string{}}
	if !reflect.DeepEqual(got.SSOPolicy, want) {
		t.Errorf("default policy = %+v, want %+v", got.SSOPolicy, want)
	}
}

func TestOAuthSSOPolicy_RoundTripKeepsClientSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	client := OAuthProviderSettings{Provider: OAuthProviderOIDC, Enabled: true, ClientID: "c", IssuerURL: "https://idp.test"}
	if err := db.UpdateOAuthProviderSettings(ctx, client); err != nil {
		t.Fatal(err)
	}
	want := OAuthSSOPolicy{GroupsClaim: "roles", SyncRoles: true, DefaultRoleID: "role_operator", JITProvisioning: true, AllowedDomains: []string{"corp.test"}}
	if err := db.SetOAuthSSOPolicy(ctx, OAuthProviderOIDC, want); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateOAuthProviderSettings(ctx, client); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetOAuthProviderSettings(ctx, OAuthProviderOIDC)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.SSOPolicy, want) {
		t.Errorf("policy = %+v, want %+v (a client settings update must not reset it)", got.SSOPolicy, want)
	}
	if err := db.SetOAuthSSOPolicy(ctx, "nope", want); err == nil {
		t.Error("SetOAuthSSOPolicy on a missing provider succeeded")
	}
}
