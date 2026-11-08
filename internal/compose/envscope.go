package compose

import "strings"

// Each compose service is its own member app, so its env never reaches siblings.
// Precedence (file, member env, branch or PR overrides) is in docs/compose-apps.md.

// MemberAppName returns the app name that holds the env of one compose service.
func MemberAppName(app, service string) string {
	return app + "-" + service
}

// ServiceKeyOf reports the compose service key for a member app of app.
func ServiceKeyOf(app, member string) (string, bool) {
	key, ok := strings.CutPrefix(member, app+"-")
	if !ok || key == "" {
		return "", false
	}
	return key, true
}
