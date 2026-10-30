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
		maxBody:      envInt64Positive(envMCPHTTPMaxBody, mcpHTTPDefaultBody),
		maxResponse:  envInt64Positive(envMCPHTTPMaxResponse, mcpHTTPDefaultResponse),
		callTimeout:  time.Duration(envPositiveInt(envMCPHTTPCallTimeout, mcpHTTPDefaultTimeout)) * time.Second,
		extraOrigins: splitOrigins(os.Getenv(envMCPHTTPOrigins)),
	}
	// Unauthenticated at the mux: serve checks the bearer token itself, plus flag, Origin and limits.
	mux.HandleFunc("POST /mcp", m.serve)
	return m
}
