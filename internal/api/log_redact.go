package api

import "net/url"

// redactURLCredentials strips userinfo from a URL before it is logged:
// operators paste private repo URLs like https://<token>@host/repo.git,
// and url.URL.Redacted only masks a password, not a bare token username.
func redactURLCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	if u.User == nil {
		return raw
	}
	u.User = url.User("redacted")
	return u.String()
}
