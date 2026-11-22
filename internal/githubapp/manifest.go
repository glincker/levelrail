// Package githubapp implements the GitHub App manifest registration
// flow, App-level JWT signing (RS256, per GitHub's own App
// authentication spec), and the small slice of the GitHub REST API this.
package githubapp

import "fmt"

// Manifest is the JSON body GitHub's manifest flow expects as the
// hidden form's "manifest" field value
// (https://github.com/settings/apps/new?state=...). Field names and
// json tags match GitHub's documented schema exactly; see this
// package's own doc comment for which fields this control plane
// populates and why.
type Manifest struct {
	Name                  string            `json:"name"`
	URL                   string            `json:"url"`
	HookAttributes        HookAttributes    `json:"hook_attributes"`
	RedirectURL           string            `json:"redirect_url"`
	SetupURL              string            `json:"setup_url"`
	Public                bool              `json:"public"`
	DefaultEvents         []string          `json:"default_events"`
	DefaultPermissions    map[string]string `json:"default_permissions"`
	RequestOAuthOnInstall bool              `json:"request_oauth_on_install"`
}

// HookAttributes is Manifest.HookAttributes. url is a required field of
// GitHub's manifest schema even when webhooks are never actually
// delivered (active: false below achieves that): see this package's own
// doc comment.
type HookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// BuildManifest builds the manifest for a fresh App registration.
func BuildManifest(appName, baseURL string, cfg ManifestConfig) Manifest {
	return Manifest{
		Name: appName,
		URL:  baseURL,
		HookAttributes: HookAttributes{
			URL:    baseURL + "/api/v1/github-app/webhook",
			Active: false,
		},
		RedirectURL:           baseURL + "/api/v1/github-app/callback",
		SetupURL:              baseURL + "/api/v1/github-app/installed",
		Public:                false,
		DefaultEvents:         cfg.DefaultEvents,
		DefaultPermissions:    cfg.DefaultPermissions,
		RequestOAuthOnInstall: false,
	}
}

// AppDisplayName derives a GitHub-App-name-safe, deployment-unique
// display name from the platform's own brand name and its public
// domain, e.g. "<brand> (deploy.example.com)". GitHub App names must be
// unique across the whole of GitHub, and brandName alone is not: every
// control plane built from this same codebase would otherwise try to
// register an App with the identical name. Appending the operator's own
// domain, which is unique by construction (DNS ownership), resolves
// that without inventing a random suffix that would make the App harder
// for the operator to recognize in their own GitHub settings.
func AppDisplayName(brandName, domain string) string {
	return fmt.Sprintf("%s (%s)", brandName, domain)
}
