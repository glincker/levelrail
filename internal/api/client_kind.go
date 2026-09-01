package api

import "strings"

// Client kind values audit_log.client_kind (migrations/0077) is normalized
// into: which surface actually made the request, not just which auth method
// it used (store.AuditEntry.ActorType already covers session-vs-token).
const (
	ClientKindCLI       = "cli"
	ClientKindDashboard = "dashboard"
	ClientKindMCP       = "mcp"
	ClientKindAPI       = "api"
)

// cliUserAgentPrefix and mcpUserAgentPrefix are the exact prefixes
// cmd/levelrail-cli/client.go and cmd/levelrail-mcp/main.go set on every
// outgoing request (apiclient.WithUserAgent), e.g. "levelrail-cli/v1.2.3".
const (
	cliUserAgentPrefix = "levelrail-cli/"
	mcpUserAgentPrefix = "levelrail-mcp/"
)

// browserUserAgentTokens are substrings present in every real desktop and
// mobile browser's User-Agent string (verified against Chrome, Firefox,
// Safari, and Edge's own formats, all of which lead with "Mozilla/5.0" and
// carry at least one of these engine/browser tokens). The web dashboard's
// fetch() calls (web/src/queries/*.ts) send whatever User-Agent the
// browser itself sets, since browsers block JS from overriding it, so this
// is the only signal available to recognize "came from our own dashboard"
// without adding a new header to every one of those call sites.
var browserUserAgentTokens = []string{"AppleWebKit/", "Gecko/", "Chrome/", "Firefox/", "Edg/", "Safari/"}

// clientKindFromUserAgent normalizes a request's raw User-Agent header into
// one of ClientKindCLI, ClientKindDashboard, ClientKindMCP, or
// ClientKindAPI. The cli/mcp prefixes are exact and unambiguous (this
// project's own clients set them); the dashboard case matches explicitly
// against real browser User-Agent shapes rather than being "everything
// that isn't cli/mcp," so a script or curl request with no recognizable
// signal correctly falls into ClientKindAPI instead of being misread as a
// dashboard request.
func clientKindFromUserAgent(userAgent string) string {
	switch {
	case strings.HasPrefix(userAgent, cliUserAgentPrefix):
		return ClientKindCLI
	case strings.HasPrefix(userAgent, mcpUserAgentPrefix):
		return ClientKindMCP
	case isBrowserUserAgent(userAgent):
		return ClientKindDashboard
	default:
		return ClientKindAPI
	}
}

// isBrowserUserAgent reports whether userAgent has the shape a real
// browser sends: "Mozilla/5.0" plus at least one engine/browser token, the
// combination every mainstream browser (Chrome, Firefox, Safari, Edge)
// includes and a bare scripting client (curl, a Go http.Client, a Python
// requests session) does not.
func isBrowserUserAgent(userAgent string) bool {
	if !strings.HasPrefix(userAgent, "Mozilla/5.0") {
		return false
	}
	for _, token := range browserUserAgentTokens {
		if strings.Contains(userAgent, token) {
			return true
		}
	}
	return false
}
