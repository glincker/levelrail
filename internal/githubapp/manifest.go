// Package githubapp implements the GitHub App manifest registration
// flow, App-level JWT signing (RS256, per GitHub's own App
// authentication spec), and the small slice of the GitHub REST API this
// control plane needs once an App is connected: minting installation
// access tokens, listing an installation's repositories, and listing a
// repository's branches.
//
// This package holds no store or internal/secrets dependency: it is a
// pure GitHub API client plus manifest/JWT construction, the same
// "narrow, single-purpose, no upward dependency" shape internal/backup's
// S3 client wrapper keeps. internal/api owns orchestration (persisting
// credentials, resolving the private key back out, deciding ability
// tiers); this package only knows how to talk to github.com and
// api.github.com.
//
// Deliberately does not implement webhook delivery handling: this
// control plane's manifest declares a webhook URL (GitHub's manifest
// schema requires hook_attributes.url) but sets hook_attributes.active
// to false, so GitHub never actually sends a delivery there. Building
// real webhook-driven auto-deploy is explicit, separate, future scope,
// not something this package pretends to support by declaring an active
// endpoint it doesn't implement.
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
// appName is the App's display name on GitHub, expected to already be
// brand-derived and deployment-unique by the caller (internal/api's
// handleStartGitHubAppRegistration appends the operator's own primary
// domain to brand.Brand.Name, since App names must be unique across all
// of GitHub and a bare brand name would collide across every operator
// running this same control plane). baseURL is this control plane's own
// public, reachable origin (https://<primary domain>, no trailing
// slash): every URL below is baseURL plus a fixed, already-routed path.
//
// Permissions requested: contents:read (clone access and branch
// listing, the two things internal/githubapp.Client actually calls) and
// metadata:read (GitHub's own baseline permission, implicitly required
// by nearly every other read scope and requestable at zero additional
// operator-visible risk). No write permissions, no webhook events
// (DefaultEvents is empty: nothing in this codebase processes a webhook
// delivery yet). Public is false: this App is registered for one
// operator's own account/org, not for other GitHub users to discover
// and install. RequestOAuthOnInstall is false: this integration only
// ever authenticates as the App/installation, never as the installing
// GitHub user, so there is no use for a user-level OAuth token.
func BuildManifest(appName, baseURL string) Manifest {
	return Manifest{
		Name: appName,
		URL:  baseURL,
		HookAttributes: HookAttributes{
			URL:    baseURL + "/api/v1/github-app/webhook",
			Active: false,
		},
		RedirectURL:   baseURL + "/api/v1/github-app/callback",
		SetupURL:      baseURL + "/api/v1/github-app/installed",
		Public:        false,
		DefaultEvents: []string{},
		DefaultPermissions: map[string]string{
			"contents": "read",
			"metadata": "read",
		},
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
