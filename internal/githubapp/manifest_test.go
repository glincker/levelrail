package githubapp

import "testing"

func TestBuildManifest_UsesConfiguredPermissionsAndEvents(t *testing.T) {
	cfg := ManifestConfig{
		DefaultPermissions: map[string]string{"contents": "read", "issues": "read"},
		DefaultEvents:      []string{"push"},
	}
	m := BuildManifest("MyApp (deploy.example.com)", "https://deploy.example.com", cfg)

	if m.DefaultPermissions["issues"] != "read" {
		t.Errorf("DefaultPermissions[issues] = %q, want %q", m.DefaultPermissions["issues"], "read")
	}
	if len(m.DefaultEvents) != 1 || m.DefaultEvents[0] != "push" {
		t.Errorf("DefaultEvents = %v, want [push]", m.DefaultEvents)
	}
}

func TestBuildManifest_HookNeverActive(t *testing.T) {
	// Regardless of DefaultEvents, HookAttributes.Active must stay
	// false: no route in this codebase handles a delivery yet
	// (BuildManifest's own doc comment). This is the one regression
	// this test exists to catch.
	cfg := ManifestConfig{DefaultEvents: []string{"push", "pull_request"}}
	m := BuildManifest("MyApp", "https://deploy.example.com", cfg)

	if m.HookAttributes.Active {
		t.Error("HookAttributes.Active = true, want false (no webhook receiver exists yet)")
	}
}

func TestBuildManifest_URLsDerivedFromBaseURL(t *testing.T) {
	m := BuildManifest("MyApp", "https://deploy.example.com", DefaultManifestConfig())

	if m.URL != "https://deploy.example.com" {
		t.Errorf("URL = %q, want %q", m.URL, "https://deploy.example.com")
	}
	if m.RedirectURL != "https://deploy.example.com/api/v1/github-app/callback" {
		t.Errorf("RedirectURL = %q, want the callback path", m.RedirectURL)
	}
	if m.SetupURL != "https://deploy.example.com/api/v1/github-app/installed" {
		t.Errorf("SetupURL = %q, want the installed path", m.SetupURL)
	}
	if m.HookAttributes.URL != "https://deploy.example.com/api/v1/github-app/webhook" {
		t.Errorf("HookAttributes.URL = %q, want the webhook path", m.HookAttributes.URL)
	}
	if m.Public {
		t.Error("Public = true, want false")
	}
	if m.RequestOAuthOnInstall {
		t.Error("RequestOAuthOnInstall = true, want false")
	}
}
