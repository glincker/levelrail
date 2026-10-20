package gitlabapp

import (
	"net/url"
	"strings"
)

// DefaultScope requests read access for project listing and write
// access for registering a project webhook: GitLab's own docs
// (docs.gitlab.com/ee/api/oauth2.html) document "api" as required for
// the webhook-creation endpoint, "read_api" alone is not enough.
const DefaultScope = "api"

// AuthorizeURL builds the GitLab OAuth2 authorization-code flow's entry
// point: a real browser redirect (GET), not a fetch call, the same
// "the operator's browser must navigate here itself" shape
// internal/githubapp's manifest flow requires for the equivalent reason.
func AuthorizeURL(instanceURL, clientID, redirectURI, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {DefaultScope},
		"state":         {state},
	}
	return strings.TrimRight(instanceURL, "/") + "/oauth/authorize?" + q.Encode()
}
