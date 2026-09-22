package giteaapp

import (
	"net/url"
	"strings"
)

// DefaultScope requests read and write repository access (needed to
// list repos/branches and to register a push webhook) plus basic user
// identification, Gitea's own OAuth2 scope names
// (docs.gitea.com/development/oauth2-provider).
const DefaultScope = "read:repository write:repository read:user"

// AuthorizeURL builds Gitea's OAuth2 authorization-code flow's entry
// point: a real browser redirect (GET), the same "the operator's browser
// must navigate here itself" shape internal/gitlabapp.AuthorizeURL's own
// doc comment describes.
func AuthorizeURL(instanceURL, clientID, redirectURI, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {DefaultScope},
		"state":         {state},
	}
	return strings.TrimRight(instanceURL, "/") + "/login/oauth/authorize?" + q.Encode()
}
