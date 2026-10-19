package api

import "github.com/GLINCKER/levelrail/internal/envredact"

// redactURLCredentials strips userinfo from a URL before it is logged:
// operators paste private repo URLs like https://<token>@host/repo.git,
// and url.URL.Redacted only masks a password, not a bare token username.
func redactURLCredentials(raw string) string {
	out, err := envredact.URLUserinfo(raw, "redacted")
	if err != nil {
		return "<unparseable url>"
	}
	return out
}
