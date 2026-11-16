package main

import (
	"context"
	"strings"
	"time"
)

// replacedLogin is the saved token a forced login is about to supersede.
type replacedLogin struct {
	name string
}

// savedLoginToReplace returns the profile's saved token only when it is not from env or flags,
// targets the same server, and the server still accepts it. Anything else is left alone.
func savedLoginToReplace(prog, profile, apiURL string, lookupEnv func(string) (string, bool)) (replacedLogin, bool) {
	if !strings.HasPrefix(credentialSource("", prog, profile), "profile") {
		return replacedLogin{}, false
	}
	creds, err := readCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" || strings.TrimRight(creds.APIURL, "/") != strings.TrimRight(apiURL, "/") {
		return replacedLogin{}, false
	}
	for _, name := range []string{envLevelrailToken, envAPIToken} {
		if v, ok := lookupEnv(name); ok && v != "" {
			return replacedLogin{}, false
		}
	}
	info, err := verifyCredential(apiURL, creds.Token, durationEnv(lookupEnv, envVerifyTimeout, defaultVerifyTimeout))
	if err != nil || info.Kind != "token" || info.Name == "" {
		return replacedLogin{}, false
	}
	return replacedLogin{name: info.Name}, true
}

// revokeReplaced best-effort revokes the superseded token, only when its name matches exactly one other live token.
func revokeReplaced(ctx context.Context, c *authSessionClient, old replacedLogin, newID string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tokens, err := c.ListTokens(ctx)
	if err != nil {
		return
	}
	var match []string
	for _, t := range tokens {
		if t.Name == old.name && t.ID != newID && t.RevokedAt == nil {
			match = append(match, t.ID)
		}
	}
	if len(match) == 1 {
		_ = c.RevokeToken(ctx, match[0])
	}
}
