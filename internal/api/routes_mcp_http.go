package api

import (
	"net/http"
	"os"
	"time"
)

func (rt *Router) registerMCPHTTPRoutes(mux *http.ServeMux) *mcpHTTP {
	m := &mcpHTTP{
		rt:           rt,
		limit:        newAPIRateLimiter(envPositiveInt(envMCPHTTPRPM, mcpHTTPDefaultRPM)),
		oauthLimit:   newAPIRateLimiter(envMCPOAuthTokenLimit()),
		authzLimit:   newAPIRateLimiter(envMCPOAuthAuthorizeLimit()),
		maxBody:      envInt64Positive(envMCPHTTPMaxBody, mcpHTTPDefaultBody),
		maxResponse:  envInt64Positive(envMCPHTTPMaxResponse, mcpHTTPDefaultResponse),
		callTimeout:  time.Duration(envPositiveInt(envMCPHTTPCallTimeout, mcpHTTPDefaultTimeout)) * time.Second,
		extraOrigins: splitOrigins(os.Getenv(envMCPHTTPOrigins)),
	}
	// Unauthenticated at the mux: serve checks the bearer token itself, plus flag, Origin and limits.
	mux.HandleFunc("POST /mcp", m.serve)
	rt.registerMCPOAuthRoutes(mux, m)
	return m
}

// registerMCPOAuthRoutes mounts the OAuth 2.1 authorization server for the
// remote MCP endpoint. Every path is behind the mcp-http feature gate.
func (rt *Router) registerMCPOAuthRoutes(mux *http.ServeMux, m *mcpHTTP) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", rt.handleMCPProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", rt.handleMCPProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", rt.handleMCPAuthServerMetadata)
	mux.HandleFunc("GET /oauth/authorize", m.handleMCPAuthorize)
	mux.HandleFunc("POST /oauth/token", m.handleMCPToken)
	mux.HandleFunc("GET /api/v1/mcp/oauth/consent/{id}", rt.requireAuth(rt.handleMCPConsentGet))
	mux.HandleFunc("POST /api/v1/mcp/oauth/consent/{id}/approve", rt.requireAuth(rt.handleMCPConsentApprove))
	mux.HandleFunc("POST /api/v1/mcp/oauth/consent/{id}/deny", rt.requireAuth(rt.handleMCPConsentDeny))
	mux.HandleFunc("GET /api/v1/mcp/oauth/clients", rt.requireAbility(AbilityRoot, rt.handleListMCPClients))
	mux.HandleFunc("POST /api/v1/mcp/oauth/clients", rt.requireAbility(AbilityRoot, rt.handleCreateMCPClient))
	mux.HandleFunc("DELETE /api/v1/mcp/oauth/clients/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteMCPClient))
}
