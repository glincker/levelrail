package bitbucketapp

import "net/url"

// authorizeURLBase is Bitbucket Cloud's fixed OAuth2 authorization
// endpoint (no per-instance host, unlike internal/gitlabapp.AuthorizeURL:
// this package is Cloud-only, see this package's own doc comment).
const authorizeURLBase = "https://bitbucket.org/site/oauth2/authorize"

// AuthorizeURL builds Bitbucket's OAuth2 authorization-code flow entry
// point: a real browser redirect (GET), the same "the operator's
// browser must navigate here itself" shape every other provider's own
// authorize/manifest entry point requires. No scope parameter: unlike
// GitLab's own AuthorizeURL, Bitbucket OAuth consumers have their
// permissions configured once on the consumer itself
// (Repositories: Read, Webhooks: Read and Write), not requested per
// authorization (docs/design/git-provider-integrations.md section 3).
func AuthorizeURL(clientID, redirectURI, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
	}
	return authorizeURLBase + "?" + q.Encode()
}
