package api

import "testing"

func TestInstallationSettingsURL(t *testing.T) {
	tests := []struct {
		name, instance, kind, login string
		id                          int64
		want                        string
	}{
		{"personal account", "https://github.com", "user", "octocat", 42, "https://github.com/settings/installations/42"},
		{"organization", "https://github.com/", "organization", "acme-corp", 7, "https://github.com/organizations/acme-corp/settings/installations/7"},
		{"enterprise server", "https://git.example.com", "organization", "eng", 9, "https://git.example.com/organizations/eng/settings/installations/9"},
		{"login is path escaped", "https://github.com", "organization", "a b", 1, "https://github.com/organizations/a%20b/settings/installations/1"},
		{"no instance url", "", "user", "octocat", 42, ""},
		{"no installation id", "https://github.com", "user", "octocat", 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := installationSettingsURL(tc.instance, tc.kind, tc.login, tc.id); got != tc.want {
				t.Errorf("installationSettingsURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
