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
	// ClientKindSystem is written directly by internal/ingress's
	// SQLiteStorage for a certificate issuance/renewal it detects, not
	// derived here: there is no User-Agent, because there is no request.
	ClientKindSystem = "system"
)

// cliUserAgentPrefix and mcpUserAgentPrefix are the exact prefixes
// cmd/levelrail-cli/client.go and cmd/levelrail-mcp/main.go set on every
// outgoing request (apiclient.WithUserAgent), e.g. "levelrail-cli/v1.2.3".
const (
	cliUserAgentPrefix = "levelrail-cli/"
	mcpUserAgentPrefix = "levelrail-mcp/"
)

// Browsers always send these tokens and JS cannot override the User-Agent, so they identify the dashboard without a new header.
var browserUserAgentTokens = []string{"AppleWebKit/", "Gecko/", "Chrome/", "Firefox/", "Edg/", "Safari/"}

// Scripts without a recognizable User-Agent fall into ClientKindAPI, not the dashboard.
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

// Bare scripting clients (curl, Go, Python) lack the browser tokens, so they are not classed as browsers.
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
